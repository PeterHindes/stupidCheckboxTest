package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
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
	conn     *websocket.Conn
	mutex    sync.Mutex
	lastSend time.Time
}

type BatchProcessor struct {
	mu            sync.Mutex
	updates       []updateMsg
	batchInterval time.Duration
	batchSize     int
	active        bool
	id            int
	workerPool    chan struct{}
	pendingWork   chan []updateMsg
}

type BatchProcessorPool struct {
	processors []*BatchProcessor
	mu         sync.RWMutex
}

type updateMsg struct {
	id     int
	state  bool
	sender *SafeWebSocket
}

func NewBatchProcessorPool(numProcs int, interval time.Duration, batchSize int) *BatchProcessorPool {
	pool := &BatchProcessorPool{
		processors: make([]*BatchProcessor, numProcs),
	}

	for i := 0; i < numProcs; i++ {
		pool.processors[i] = NewBatchProcessor(interval, batchSize, i)
	}
	return pool
}

func (pool *BatchProcessorPool) addUpdate(id int, state bool, sender *SafeWebSocket) {
	// Use checkbox ID to determine processor (ensures same checkbox always goes to same processor)
	procIdx := id % len(pool.processors)
	pool.processors[procIdx].addUpdate(id, state, sender)
}

func NewBatchProcessor(interval time.Duration, batchSize int, id int) *BatchProcessor {
	bp := &BatchProcessor{
		updates:       make([]updateMsg, 0, batchSize),
		batchInterval: interval,
		batchSize:     batchSize,
		active:        true,
		id:            id,
		workerPool:    make(chan struct{}, runtime.NumCPU()),
		pendingWork:   make(chan []updateMsg, 100),
	}
	go bp.processLoop()
	return bp
}

func (bp *BatchProcessor) processLoop() {
	ticker := time.NewTicker(bp.batchInterval)
	defer ticker.Stop()

	for bp.active {
		select {
		case <-ticker.C:
			bp.collectBatch()
		case updates := <-bp.pendingWork:
			bp.processBatch(updates)
		}
	}
}

func (bp *BatchProcessor) collectBatch() {
	bp.mu.Lock()
	if len(bp.updates) == 0 {
		bp.mu.Unlock()
		return
	}

	updates := bp.updates
	bp.updates = make([]updateMsg, 0, bp.batchSize)
	bp.mu.Unlock()

	select {
	case bp.pendingWork <- updates:
	default:
		// If channel is full, process immediately
		bp.processBatch(updates)
	}
}

func (bp *BatchProcessor) processBatch(updates []updateMsg) {
	// Consolidate updates (keep only latest state per checkbox)
	consolidated := make(map[int]bool)
	for _, update := range updates {
		consolidated[update.id] = update.state
	}

	// Convert to binary messages
	messages := make([][]byte, 0, len(consolidated))
	for id, state := range consolidated {
		msg := make([]byte, 5)
		msg[0] = 0
		msg[1] = byte(id >> 16)
		msg[2] = byte(id >> 8)
		msg[3] = byte(id)
		if state {
			msg[4] = 1
		}
		messages = append(messages, msg)
	}

	// Get current clients in chunks
	clientsMutex.RLock()
	activeClients := make([]*SafeWebSocket, 0, len(clients))
	for client := range clients {
		activeClients = append(activeClients, client)
	}
	clientsMutex.RUnlock()

	// Process clients in parallel with worker pool
	var wg sync.WaitGroup
	chunkSize := 100 // Increased chunk size for better throughput

	for i := 0; i < len(activeClients); i += chunkSize {
		end := i + chunkSize
		if end > len(activeClients) {
			end = len(activeClients)
		}

		wg.Add(1)
		go func(clients []*SafeWebSocket) {
			defer wg.Done()
			bp.workerPool <- struct{}{}        // Acquire worker
			defer func() { <-bp.workerPool }() // Release worker

			for _, client := range clients {
				client.mutex.Lock()
				now := time.Now()
				// Enforce 50ms rate limit per client
				if now.Sub(client.lastSend) < 50*time.Millisecond {
					client.mutex.Unlock()
					continue
				}
				client.lastSend = now

				client.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				for _, msg := range messages {
					if err := client.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
						client.mutex.Unlock()
						go removeClient(client)
						break
					}
				}
				client.conn.SetWriteDeadline(time.Time{})
				client.mutex.Unlock()
			}
		}(activeClients[i:end])
	}

	wg.Wait()
}

func (bp *BatchProcessor) addUpdate(id int, state bool, sender *SafeWebSocket) {
	bp.mu.Lock()
	bp.updates = append(bp.updates, updateMsg{id: id, state: state, sender: sender})
	bp.mu.Unlock()
}

var (
	settings = Settings{
		BoardSize: 1000000,
	}
	bitArray           []bool
	clients            = make(map[*SafeWebSocket]bool)
	clientsMutex       sync.RWMutex
	batchProcessorPool *BatchProcessorPool
	writeTimeout       = 1 * time.Second // Increased for large batches
	upgrader           = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// Increase buffer sizes and timeouts for large number of clients
		HandshakeTimeout: 10 * time.Second,
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

func broadcastChange(sender *SafeWebSocket, id int, state bool) {
	batchProcessorPool.addUpdate(id, state, sender)
}

func broadcastToAll() {
	clientsMutex.RLock()
	activeClients := make([]*SafeWebSocket, 0, len(clients))
	for client := range clients {
		activeClients = append(activeClients, client)
	}
	clientsMutex.RUnlock()

	bytes := make([]byte, (settings.BoardSize+7)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}

	// Process clients in chunks to avoid overwhelming network
	chunkSize := 20
	for i := 0; i < len(activeClients); i += chunkSize {
		end := i + chunkSize
		if end > len(activeClients) {
			end = len(activeClients)
		}

		for _, client := range activeClients[i:end] {
			client.mutex.Lock()
			client.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
			err := client.conn.WriteMessage(websocket.BinaryMessage, bytes)
			client.conn.SetWriteDeadline(time.Time{})
			client.mutex.Unlock()

			if err != nil {
				logError("Error broadcasting to client: %v", err)
				go removeClient(client)
			}
		}

		// Small pause between chunks
		time.Sleep(time.Millisecond)
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
	safeConn.mutex.Lock()
	err = sendBitArrayToClient(conn) // Use the helper function to send the full board state
	safeConn.mutex.Unlock()
	if err != nil {
		logError("Error sending initial state: %v", err)
		return
	}

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

func init() {
	// Increase system limits for many connections
	if err := setSystemLimits(); err != nil {
		log.Printf("Warning: Could not set system limits: %v", err)
	}
}

func setSystemLimits() error {
	// These are handled by the OS, just a placeholder in case we need to add limits later
	return nil
}

func main() {
	// Parse command line flags
	resetBoard := flag.Bool("reset", false, "Reset the board to empty state on startup")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	batchInterval := flag.Duration("batch-interval", 2*time.Millisecond, "Interval for batching updates")
	batchSize := flag.Int("batch-size", 10000, "Size of each batch")
	numProcessors := flag.Int("processors", runtime.NumCPU(), "Number of batch processors")
	flag.Parse()

	// Setup loggers
	errorLog = log.New(os.Stderr, "ERROR: ", log.Ltime)
	if *verbose {
		infoLog = log.New(os.Stdout, "INFO: ", log.Ltime)
	} else {
		infoLog = log.New(io.Discard, "", 0)
	}

	// Initialize batch processor pool
	batchProcessorPool = NewBatchProcessorPool(*numProcessors, *batchInterval, *batchSize)

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

	addr := "127.0.0.1:1335"
	logInfo("Server starting. Access at:")
	logInfo("    http://%s", addr)
	logInfo("Board size: %d", settings.BoardSize)
	logInfo("Batch interval: %v, Batch size: %d", *batchInterval, *batchSize)

	if err := http.ListenAndServe(addr, nil); err != nil {
		logError("%v", err)
		os.Exit(1)
	}
}
