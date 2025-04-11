package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	numClients        = flag.Int("clients", 1000, "number of concurrent clients")
	togglesPerClient  = flag.Int("toggles", 10000, "number of toggles per client")
	toggleInterval    = flag.Duration("interval", 10*time.Millisecond, "interval between toggles")
	serverAddr        = flag.String("server", "127.0.0.1:1335", "server address")
	resultsChannel    = make(chan clientResult, 1000)
	connectionsActive = 0
	mu                sync.Mutex
)

type clientResult struct {
	clientID      int
	toggleCount   int
	totalDuration time.Duration
	errors        int
}

func runClient(clientID int, wg *sync.WaitGroup) {
	defer wg.Done()

	// Create a random source for this client
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)))

	u := url.URL{Scheme: "ws", Host: *serverAddr, Path: "/ws"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("Client %d dial error: %v", clientID, err)
		resultsChannel <- clientResult{clientID: clientID, errors: *togglesPerClient}
		return
	}
	defer c.Close()

	mu.Lock()
	connectionsActive++
	currentConnections := connectionsActive
	mu.Unlock()
	log.Printf("Client %d connected (total active: %d)", clientID, currentConnections)

	// Wait for initial board size message
	_, msg, err := c.ReadMessage()
	if err != nil {
		log.Printf("Client %d initial message error: %v", clientID, err)
		resultsChannel <- clientResult{clientID: clientID, errors: *togglesPerClient}
		return
	}

	// Parse board size from message
	boardSize := 1000000 // Default size in case of parsing error
	if len(msg) == 5 && msg[0] == 1 {
		boardSize = (int(msg[1]) << 24) | (int(msg[2]) << 16) | (int(msg[3]) << 8) | int(msg[4])
	}

	// Wait for initial board state
	_, _, err = c.ReadMessage()
	if err != nil {
		log.Printf("Client %d initial state error: %v", clientID, err)
		resultsChannel <- clientResult{clientID: clientID, errors: *togglesPerClient}
		return
	}

	start := time.Now()
	errors := 0

	for i := 0; i < *togglesPerClient; i++ {
		// Create toggle message with random checkbox
		msg := make([]byte, 5)
		msg[0] = 0 // type 0 = checkbox change
		index := rnd.Intn(boardSize)
		msg[1] = byte(index >> 16)
		msg[2] = byte(index >> 8)
		msg[3] = byte(index)
		msg[4] = byte(rnd.Intn(2)) // Random state (0 or 1)

		err := c.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			errors++
			continue
		}

		time.Sleep(*toggleInterval)
	}

	duration := time.Since(start)
	resultsChannel <- clientResult{
		clientID:      clientID,
		toggleCount:   *togglesPerClient,
		totalDuration: duration,
		errors:        errors,
	}

	mu.Lock()
	connectionsActive--
	mu.Unlock()
}

func main() {
	flag.Parse()

	var wg sync.WaitGroup
	start := time.Now()

	// Start all clients
	for i := 0; i < *numClients; i++ {
		wg.Add(1)
		go runClient(i, &wg)
		time.Sleep(50 * time.Millisecond) // Stagger client connections
	}

	// Start result collector
	go func() {
		wg.Wait()
		close(resultsChannel)
	}()

	// Collect and aggregate results
	var totalToggles, totalErrors int
	var totalDuration time.Duration
	maxDuration := time.Duration(0)

	for result := range resultsChannel {
		totalToggles += result.toggleCount
		totalErrors += result.errors
		totalDuration += result.totalDuration
		if result.totalDuration > maxDuration {
			maxDuration = result.totalDuration
		}
	}

	// Calculate and print statistics
	overallDuration := time.Since(start)
	avgDuration := totalDuration / time.Duration(*numClients)
	togglesPerSecond := float64(totalToggles-totalErrors) / overallDuration.Seconds()

	fmt.Printf("\nBenchmark Results:\n")
	fmt.Printf("================\n")
	fmt.Printf("Total Clients: %d\n", *numClients)
	fmt.Printf("Toggles per Client: %d\n", *togglesPerClient)
	fmt.Printf("Total Toggles: %d\n", totalToggles)
	fmt.Printf("Successful Toggles: %d\n", totalToggles-totalErrors)
	fmt.Printf("Errors: %d\n", totalErrors)
	fmt.Printf("Overall Duration: %v\n", overallDuration)
	fmt.Printf("Average Client Duration: %v\n", avgDuration)
	fmt.Printf("Max Client Duration: %v\n", maxDuration)
	fmt.Printf("Toggles per Second: %.2f\n", togglesPerSecond)
}
