package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

type Settings struct {
	BoardSize int `json:"boardSize"`
}

type Message struct {
	Type  string `json:"type,omitempty"`
	ID    int    `json:"id,omitempty"`
	State bool   `json:"state,omitempty"`
}

var (
	settings = Settings{
		BoardSize: 1000000, // Default board size
	}
	bitArray []bool
	clients  = make(map[*websocket.Conn]bool)
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
)

func loadSettings() error {
	data, err := os.ReadFile("settings.json")
	if err != nil {
		if os.IsNotExist(err) {
			// Create default settings file if it doesn't exist
			defaultSettings := Settings{BoardSize: 1000000}
			data, err := json.MarshalIndent(defaultSettings, "", "    ")
			if err != nil {
				return fmt.Errorf("error creating default settings: %v", err)
			}
			if err := os.WriteFile("settings.json", data, 0644); err != nil {
				return fmt.Errorf("error writing default settings: %v", err)
			}
			return nil
		}
		return fmt.Errorf("error reading settings: %v", err)
	}

	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("error parsing settings: %v", err)
	}

	// Initialize bitArray with configured size
	bitArray = make([]bool, settings.BoardSize)
	return nil
}

func sendBitArrayToClient(conn *websocket.Conn) error {
	bytes := make([]byte, (settings.BoardSize+7)/8) // Calculate the number of bytes needed
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}
	return conn.WriteMessage(websocket.BinaryMessage, bytes)
}

func broadcastToAll() {
	bytes := make([]byte, (settings.BoardSize+7)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}

	for client := range clients {
		err := client.WriteMessage(websocket.BinaryMessage, bytes)
		if err != nil {
			log.Printf("Error broadcasting to client: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
}

func broadcastChange(sender *websocket.Conn, id int, state bool) {
	// Create 5-byte message: type (1 byte) + index (3 bytes) + state (1 byte)
	msg := make([]byte, 5)
	msg[0] = 0 // type 0 = checkbox change
	msg[1] = byte(id >> 16)
	msg[2] = byte(id >> 8)
	msg[3] = byte(id)
	if state {
		msg[4] = 1
	}

	for client := range clients {
		if client != sender {
			if err := client.WriteMessage(websocket.BinaryMessage, msg); err != nil {
				log.Printf("Error broadcasting to client: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Error upgrading to WebSocket: %v", err)
		return
	}

	// Register the client
	clients[conn] = true
	defer func() {
		delete(clients, conn)
		conn.Close()
	}()

	// Send board size as 4 bytes
	sizeMsg := make([]byte, 5)
	sizeMsg[0] = 1 // type 1 = board size
	sizeMsg[1] = byte(settings.BoardSize >> 24)
	sizeMsg[2] = byte(settings.BoardSize >> 16)
	sizeMsg[3] = byte(settings.BoardSize >> 8)
	sizeMsg[4] = byte(settings.BoardSize)
	if err := conn.WriteMessage(websocket.BinaryMessage, sizeMsg); err != nil {
		log.Printf("Error sending board size: %v", err)
		return
	}

	// Send initial board state
	if err := sendBitArrayToClient(conn); err != nil {
		log.Printf("Error sending initial state: %v", err)
		return
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		if len(message) == 5 && message[0] == 0 { // checkbox change message
			id := int(message[1])<<16 | int(message[2])<<8 | int(message[3])
			state := message[4] != 0

			if id >= 0 && id < settings.BoardSize {
				bitArray[id] = state
				log.Printf("Checkbox %d set to %v", id, state)
				broadcastChange(conn, id, state)
			}
		}
	}
}

func saveToDisk() {
	bytes := make([]byte, (settings.BoardSize+7)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}
	if err := os.WriteFile("bitArray.bin", bytes, 0644); err != nil {
		log.Printf("Error saving to disk: %v", err)
	} else {
		log.Printf("Board state saved to disk")
	}
}

func loadFromDisk() {
	bytes, err := os.ReadFile("bitArray.bin")
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("Error loading from disk: %v", err)
		}
		return
	}

	maxBits := len(bytes) * 8
	if maxBits > settings.BoardSize {
		maxBits = settings.BoardSize
	}

	for i := 0; i < maxBits && i < len(bitArray); i++ {
		bitArray[i] = (bytes[i/8] & (1 << uint(7-(i%8)))) != 0
	}
}

func main() {
	// Load settings
	if err := loadSettings(); err != nil {
		log.Fatal(err)
	}

	// Load initial state
	loadFromDisk()

	// Start periodic save and broadcast
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		for range ticker.C {
			saveToDisk()
			broadcastToAll()
		}
	}()

	// Setup HTTP server
	http.HandleFunc("/ws", handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("static")))

	addr := ":1335"
	log.Printf("Server starting. Access at:\n")
	log.Printf("    http://localhost%s\n", addr)
	log.Printf("Board size: %d\n", settings.BoardSize)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
