package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
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

type SafeWebSocket struct {
	conn  *websocket.Conn
	mutex sync.Mutex
}

var (
	settings = Settings{
		BoardSize: 1000000, // Default board size
	}
	bitArray     []bool
	clients      = make(map[*SafeWebSocket]bool)
	clientsMutex sync.RWMutex
	upgrader     = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	infoLog  *log.Logger
	errorLog *log.Logger
)

func logInfo(format string, v ...interface{}) {
	if infoLog != nil {
		infoLog.Printf(format, v...)
	}
}

func logError(format string, v ...interface{}) {
	errorLog.Printf(format, v...)
}

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

	clientsMutex.RLock()
	activeClients := make([]*SafeWebSocket, 0, len(clients))
	for client := range clients {
		activeClients = append(activeClients, client)
	}
	clientsMutex.RUnlock()

	for _, client := range activeClients {
		client.mutex.Lock()
		err := client.conn.WriteMessage(websocket.BinaryMessage, bytes)
		client.mutex.Unlock()
		if err != nil {
			logError("Error broadcasting to client: %v", err)
			go removeClient(client)
		}
	}
}

func broadcastChange(sender *SafeWebSocket, id int, state bool) {
	// Create 5-byte message: type (1 byte) + index (3 bytes) + state (1 byte)
	msg := make([]byte, 5)
	msg[0] = 0 // type 0 = checkbox change
	msg[1] = byte(id >> 16)
	msg[2] = byte(id >> 8)
	msg[3] = byte(id)
	if state {
		msg[4] = 1
	}

	clientsMutex.RLock()
	activeClients := make([]*SafeWebSocket, 0, len(clients))
	for client := range clients {
		if client != sender {
			activeClients = append(activeClients, client)
		}
	}
	clientsMutex.RUnlock()

	for _, client := range activeClients {
		client.mutex.Lock()
		err := client.conn.WriteMessage(websocket.BinaryMessage, msg)
		client.mutex.Unlock()
		if err != nil {
			logError("Error broadcasting to client: %v", err)
			go removeClient(client)
		}
	}
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logError("Error upgrading to WebSocket: %v", err)
		return
	}

	safeConn := &SafeWebSocket{
		conn: conn,
	}

	// Register the client
	clientsMutex.Lock()
	clients[safeConn] = true
	clientCount := len(clients)
	clientsMutex.Unlock()

	logInfo("New client connected. Total clients: %d", clientCount)

	defer removeClient(safeConn)

	// Send board size as 4 bytes
	sizeMsg := make([]byte, 5)
	sizeMsg[0] = 1 // type 1 = board size
	sizeMsg[1] = byte(settings.BoardSize >> 24)
	sizeMsg[2] = byte(settings.BoardSize >> 16)
	sizeMsg[3] = byte(settings.BoardSize >> 8)
	sizeMsg[4] = byte(settings.BoardSize)

	safeConn.mutex.Lock()
	if err := conn.WriteMessage(websocket.BinaryMessage, sizeMsg); err != nil {
		safeConn.mutex.Unlock()
		logError("Error sending board size: %v", err)
		return
	}
	safeConn.mutex.Unlock()

	// Send initial board state
	bytes := make([]byte, (settings.BoardSize+7)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}

	safeConn.mutex.Lock()
	if err := conn.WriteMessage(websocket.BinaryMessage, bytes); err != nil {
		safeConn.mutex.Unlock()
		logError("Error sending initial state: %v", err)
		return
	}
	safeConn.mutex.Unlock()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logError("WebSocket read error: %v", err)
			}
			return
		}

		if len(message) == 5 && message[0] == 0 { // checkbox change message
			id := int(message[1])<<16 | int(message[2])<<8 | int(message[3])
			state := message[4] != 0

			if id >= 0 && id < settings.BoardSize {
				bitArray[id] = state
				logInfo("Checkbox %d set to %v", id, state)
				broadcastChange(safeConn, id, state)
			}
		}
	}
}

func removeClient(client *SafeWebSocket) {
	clientsMutex.Lock()
	defer clientsMutex.Unlock()
	if _, exists := clients[client]; exists {
		client.conn.Close()
		delete(clients, client)
		logInfo("Client disconnected. Total clients: %d", len(clients))
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
		logError("Error saving to disk: %v", err)
	} else {
		logInfo("Board state saved to disk")
	}
}

func loadFromDisk() {
	bytes, err := os.ReadFile("bitArray.bin")
	if err != nil {
		if !os.IsNotExist(err) {
			logError("Error loading from disk: %v", err)
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
	// Parse command line flags
	resetBoard := flag.Bool("reset", false, "Reset the board to empty state on startup")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	flag.Parse()

	// Setup loggers
	errorLog = log.New(os.Stderr, "ERROR: ", log.Ltime)
	if *verbose {
		infoLog = log.New(os.Stdout, "INFO: ", log.Ltime)
	} else {
		infoLog = log.New(io.Discard, "", 0)
	}

	// Load settings
	if err := loadSettings(); err != nil {
		logError("%v", err)
		os.Exit(1)
	}

	if *resetBoard {
		logInfo("Resetting board to empty state...")
		os.Remove("bitArray.bin")
	} else {
		loadFromDisk()
	}

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

	addr := "127.0.0.1:1335"  // Using explicit IPv4 localhost
	logInfo("Server starting. Access at:")
	logInfo("    http://%s", addr)
	logInfo("Board size: %d", settings.BoardSize)

	if err := http.ListenAndServe(addr, nil); err != nil {
		logError("%v", err)
		os.Exit(1)
	}
}
