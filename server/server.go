package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)
func handleWebSocket(w http.ResponseWriter, r *http.Request, bitArray *[1000000]bool) {
	// Allow all origins
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Failed to upgrade connection to WebSocket:", err)
		return
	}
	defer conn.Close()

	// Send bitArray periodically
	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			err := sendBitArray(conn, bitArray[:])
			if err != nil {
				log.Println("Failed to send bitArray:", err)
				break
			}
		}
	}()

	// Read messages from WebSocket
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Failed to read message from WebSocket:", err)
			break
		}
		log.Println("Received message:", string(message))

		// Decode the message
		var data struct {
			ID    int  `json:"id"`
			State bool `json:"state"`
		}
		err = json.Unmarshal(message, &data)
		if err != nil {
			log.Println("Failed to decode message:", err)
			continue
		}

		// Modify the boolean array based on the decoded message
		if data.ID >= 0 && data.ID < len(bitArray) {
			bitArray[data.ID] = data.State
		}
	}

}
func sendBitArray(conn *websocket.Conn, bitArray []bool) error {
	// Encode the bitArray into a binary message without json
	// convert bitArray to byte array
	byteArray := make([]byte, len(bitArray)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			byteArray[i/8] |= 1 << uint8(7-i%8)
		}
	}

	// Send the bitArray
	err := conn.WriteMessage(websocket.BinaryMessage, byteArray)
	if err != nil {
		return err
	}

	return nil
}

var bitArray [1000000]bool

func main() {

	// Initialize the bitArray with values from a file
	err := loadBinaryFile(&bitArray, "bitArray.bin")
	if err != nil {
		fmt.Println("Failed to load bitArray from file:", err)
		os.Exit(1)
	}


	
	// Before stoping save the bitArray to a file when the program is stopped with ctrl+c
	// Before stopping, save the bitArray to a file when the program is stopped with ctrl+c
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c

		// Save the bitArray to a file
		saveBinaryFile(bitArray[:], "bitArray.bin")
		log.Println("BitArray saved to bitArray.bin")

		os.Exit(0)
	}()

	// Register WebSocket handler
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocket(w, r, &bitArray)
	})

	// Serve static files
	http.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("static"))))

	// Start server
	addr := ":1335"
	log.Println("Starting server on", addr)
	errsrv := http.ListenAndServe(addr, nil)
	if errsrv != nil {
		log.Fatal("Failed to start server:", errsrv)
	}

	
}

func saveBinaryFile(array []bool, filename string) {
    // Create a new file
    file, err := os.Create(filename)
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    defer file.Close()

    // Save the array to the file
    for i := 0; i < len(array); i += 8 {
        microarray := make([]byte, 1)
        for j := 0; j < 8 && i+j < len(array); j++ {
            if array[i+j] {
                microarray[0] |= 1 << uint8(7-j)
            }
        }
        _, err := file.Write(microarray)
        if err != nil {
            fmt.Println("Error:", err)
            return
        }
    }
}

func loadBinaryFile(array *[1000000]bool, filename string) error {
	// Open the file
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the file
	for i := 0; i < len(array); i += 8 {
		byteArray := make([]byte, 1)
		_, err := file.Read(byteArray)
		if err != nil {
			return err
		}

		for j := 0; j < 8 && i+j < len(array); j++ {
			bit := (byteArray[0] >> uint8(7-j)) & 1
			array[i+j] = bit == 1
		}
	}
	return nil
}