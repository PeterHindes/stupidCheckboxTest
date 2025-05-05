package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Command line flags
var (
	numClients       = flag.Int("clients", 1000, "number of concurrent clients")
	togglesPerClient = flag.Int("toggles", 10000, "number of toggles per client")
	toggleInterval   = flag.Duration("interval", 10*time.Millisecond, "interval between toggles")
	serverAddr       = flag.String("server", "localhost:1335", "server address")
	secure           = flag.Bool("secure", false, "use secure WebSocket (wss://)")
	wsPath           = flag.String("path", "/ws", "WebSocket path")
	maxRetries       = flag.Int("retries", 3, "maximum connection retry attempts")
	connectionTimeout = flag.Duration("timeout", 5*time.Second, "connection timeout")
	verbose          = flag.Bool("verbose", false, "enable verbose logging")
	duration         = flag.Duration("duration", 0, "maximum test duration (0 for unlimited)")
)

// Result and state tracking
var (
	resultsChannel    = make(chan clientResult, 5000)
	connectionsActive int32
	runningTest      = int32(1)
	boardSize        = int32(1000000) // Default, will be updated
)

// Client result structure
type clientResult struct {
	clientID       int
	attemptedCount int
	successCount   int
	totalDuration  time.Duration
	errors         int
	connectErrors  int
	rateLimited    int
}

// Rate limiting for checkboxes
type clientState struct {
	lastToggle sync.Map
	mu         sync.Mutex
}

// Check if we can toggle a checkbox (rate limiting)
func (cs *clientState) canToggle(checkboxId int) bool {
	now := time.Now()
	
	if val, ok := cs.lastToggle.Load(checkboxId); ok {
		lastTime := val.(time.Time)
		if now.Sub(lastTime) < 50*time.Millisecond {
			return false
		}
	}
	
	cs.lastToggle.Store(checkboxId, now)
	return true
}

// Run a single client
func runClient(clientID int, wg *sync.WaitGroup) {
	defer wg.Done()

	// Create client state
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() + int64(clientID)))
	state := &clientState{}

	// Determine protocol
	scheme := "ws"
	if *secure {
		scheme = "wss"
	}

	u := url.URL{Scheme: scheme, Host: *serverAddr, Path: *wsPath}
	if *verbose {
		log.Printf("Client %d connecting to %s", clientID, u.String())
	}

	// Setup dialer with timeout
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = *connectionTimeout

	// Connect with retries
	var c *websocket.Conn
	var err error
	connectErrors := 0

	for retry := 0; retry < *maxRetries; retry++ {
		c, _, err = dialer.Dial(u.String(), nil)
		if err == nil {
			break
		}

		connectErrors++
		log.Printf("Client %d dial error (attempt %d/%d): %v", 
			clientID, retry+1, *maxRetries, err)

		if retry < *maxRetries-1 {
			// Exponential backoff
			backoff := time.Duration(100*(1<<retry)) * time.Millisecond
			time.Sleep(backoff)
		}
	}

	if err != nil {
		log.Printf("Client %d failed to connect after %d attempts", clientID, *maxRetries)
		resultsChannel <- clientResult{
			clientID:      clientID, 
			errors:        *togglesPerClient,
			connectErrors: connectErrors,
		}
		return
	}
	defer c.Close()

	// Update active connection count
	conns := atomic.AddInt32(&connectionsActive, 1)
	if *verbose {
		log.Printf("Client %d connected (total active: %d)", clientID, conns)
	}
	defer atomic.AddInt32(&connectionsActive, -1)

	// Wait for initial board size message
	_, msg, err := c.ReadMessage()
	if err != nil {
		log.Printf("Client %d initial message error: %v", clientID, err)
		resultsChannel <- clientResult{
			clientID:      clientID, 
			errors:        *togglesPerClient,
			connectErrors: connectErrors,
		}
		return
	}

	// Parse board size
	localBoardSize := 1000000 // Default fallback
	if len(msg) == 5 && msg[0] == 1 {
		localBoardSize = (int(msg[1]) << 24) | (int(msg[2]) << 16) | (int(msg[3]) << 8) | int(msg[4])
		atomic.StoreInt32(&boardSize, int32(localBoardSize))
		if *verbose {
			log.Printf("Client %d received board size: %d", clientID, localBoardSize)
		}
	}

	// Wait for initial board state
	_, _, err = c.ReadMessage()
	if err != nil {
		log.Printf("Client %d initial state error: %v", clientID, err)
		resultsChannel <- clientResult{
			clientID:      clientID, 
			errors:        *togglesPerClient,
			connectErrors: connectErrors,
		}
		return
	}

	// Start toggling checkboxes
	start := time.Now()
	errors := 0
	attemptedCount := 0
	successCount := 0
	rateLimited := 0
	endTime := time.Time{}
	
	if *duration > 0 {
		endTime = start.Add(*duration)
	}

	// Keep toggling until we reach the target or duration expires
	for i := 0; i < *togglesPerClient && atomic.LoadInt32(&runningTest) == 1; i++ {
		if !endTime.IsZero() && time.Now().After(endTime) {
			break
		}
		
		// Pick random checkbox
		index := rnd.Intn(localBoardSize)

		// Check rate limit
		if !state.canToggle(index) {
			rateLimited++
			time.Sleep(5 * time.Millisecond)
			continue
		}

		// Count the attempt
		attemptedCount++

		// Create toggle message
		msg := make([]byte, 5)
		msg[0] = 0 // type 0 = checkbox change
		msg[1] = byte(index >> 16)
		msg[2] = byte(index >> 8)
		msg[3] = byte(index)
		msg[4] = byte(rnd.Intn(2)) // Random state (0 or 1)

		// Send the toggle
		err := c.WriteMessage(websocket.BinaryMessage, msg)
		if err != nil {
			errors++
			if *verbose {
				log.Printf("Client %d toggle error: %v", clientID, err)
			}
			
			// If connection is broken, stop the client
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				break
			}
			continue
		}

		// Count success and wait
		successCount++
		time.Sleep(*toggleInterval)
	}

	// Send result
	duration := time.Since(start)
	resultsChannel <- clientResult{
		clientID:       clientID,
		attemptedCount: attemptedCount,
		successCount:   successCount,
		totalDuration:  duration,
		errors:         errors,
		connectErrors:  connectErrors,
		rateLimited:    rateLimited,
	}
}

func main() {
	flag.Parse()
	
	log.Printf("Starting benchmark against %s using %s protocol", 
		*serverAddr, map[bool]string{true: "wss", false: "ws"}[*secure])
	log.Printf("Clients: %d, Toggles per client: %d, Interval: %v", 
		*numClients, *togglesPerClient, *toggleInterval)
	
	if *duration > 0 {
		log.Printf("Maximum test duration: %v", *duration)
	}

	// Setup signal handling for graceful exit
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		log.Println("Received termination signal, stopping test...")
		atomic.StoreInt32(&runningTest, 0)
	}()

	var wg sync.WaitGroup
	start := time.Now()

	// Start clients
	for i := 0; i < *numClients; i++ {
		wg.Add(1)
		go runClient(i, &wg)
		time.Sleep(20 * time.Millisecond) // Less aggressive staggering
	}

	// Start result collector
	go func() {
		wg.Wait()
		close(resultsChannel)
	}()

	// Set a maximum test duration if specified
	if *duration > 0 {
		go func() {
			time.Sleep(*duration)
			log.Printf("Maximum test duration reached (%v), stopping test...", *duration)
			atomic.StoreInt32(&runningTest, 0)
		}()
	}

	// Collect results
	var totalAttempted, successfulToggles, totalErrors, totalRateLimited, connectErrors int
	var totalDuration time.Duration
	maxDuration := time.Duration(0)
	clientsReporting := 0

	for result := range resultsChannel {
		clientsReporting++
		totalAttempted += result.attemptedCount
		successfulToggles += result.successCount
		totalErrors += result.errors
		connectErrors += result.connectErrors
		totalRateLimited += result.rateLimited
		totalDuration += result.totalDuration
		
		if result.totalDuration > maxDuration {
			maxDuration = result.totalDuration
		}

		// Progress reporting for long tests
		if clientsReporting%100 == 0 {
			log.Printf("Progress: %d/%d clients reported results", clientsReporting, *numClients)
		}
	}

	// Calculate statistics
	overallDuration := time.Since(start)
	
	// Avoid division by zero
	var avgDuration time.Duration
	if clientsReporting > 0 {
		avgDuration = totalDuration / time.Duration(clientsReporting)
	}
	
	togglesPerSecond := 0.0
	if overallDuration.Seconds() > 0 {
		togglesPerSecond = float64(successfulToggles) / overallDuration.Seconds()
	}
	
	// Calculate success rate (avoiding division by zero)
	successRate := 0.0
	if totalAttempted > 0 {
		successRate = 100 * float64(successfulToggles) / float64(totalAttempted)
	}

	// Print results
	fmt.Printf("\nBenchmark Results:\n")
	fmt.Printf("================\n")
	fmt.Printf("Total Clients: %d (Reported: %d)\n", *numClients, clientsReporting)
	fmt.Printf("Board Size: %d\n", atomic.LoadInt32(&boardSize))
	fmt.Printf("Toggles per Client: %d\n", *togglesPerClient)
	fmt.Printf("Total Toggles Attempted: %d\n", totalAttempted)
	fmt.Printf("Successful Toggles: %d\n", successfulToggles)
	fmt.Printf("Connect Errors: %d\n", connectErrors)
	fmt.Printf("Toggle Errors: %d\n", totalErrors)
	fmt.Printf("Rate Limited Toggles: %d\n", totalRateLimited)
	fmt.Printf("Toggle Success Rate: %.2f%%\n", successRate)
	fmt.Printf("Overall Duration: %v\n", overallDuration)
	fmt.Printf("Average Client Duration: %v\n", avgDuration)
	fmt.Printf("Max Client Duration: %v\n", maxDuration)
	fmt.Printf("Toggles per Second: %.2f\n", togglesPerSecond)
}