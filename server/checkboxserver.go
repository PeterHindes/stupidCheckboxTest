package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Configuration
type Settings struct {
	BoardSize int `json:"boardSize"`
}

// Client connection handling
type SafeWebSocket struct {
	conn     *websocket.Conn
	mutex    sync.Mutex
	lastSend time.Time
	id       uint64
}

// Update message
type updateMsg struct {
	id     int
	state  bool
	sender *SafeWebSocket
}

// Batch processor for handling checkbox updates
type BatchProcessor struct {
	mu            sync.Mutex
	updates       []updateMsg
	batchInterval time.Duration
	batchSize     int
	active        bool
	id            int
	workerPool    chan struct{}
	pendingWork   chan []updateMsg
	done          chan struct{}
}

// Pool of batch processors
type BatchProcessorPool struct {
	processors []*BatchProcessor
	count      int
}

// Global variables
var (
	settings = Settings{
		BoardSize: 1000000,
	}
	bitArray           []bool
	clients            = make(map[*SafeWebSocket]bool)
	clientsMutex       sync.RWMutex
	batchProcessorPool *BatchProcessorPool
	writeTimeout       = 2 * time.Second
	clientIdCounter    uint64
	upgrader           = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
		ReadBufferSize:    4096,
		WriteBufferSize:   4096,
		HandshakeTimeout:  10 * time.Second,
		EnableCompression: false,
	}
	infoLog    *log.Logger
	errorLog   *log.Logger
	activeConn int32
)

// Create a new batch processor pool
func NewBatchProcessorPool(numProcs int, interval time.Duration, batchSize int) *BatchProcessorPool {
	pool := &BatchProcessorPool{
		processors: make([]*BatchProcessor, numProcs),
		count:      numProcs,
	}

	for i := 0; i < numProcs; i++ {
		pool.processors[i] = NewBatchProcessor(interval, batchSize, i)
	}
	return pool
}

// Add an update to the appropriate processor in the pool
func (pool *BatchProcessorPool) addUpdate(id int, state bool, sender *SafeWebSocket) {
	procIdx := id % pool.count
	pool.processors[procIdx].addUpdate(id, state, sender)
}

// Shutdown the processor pool
func (pool *BatchProcessorPool) shutdown() {
	for _, proc := range pool.processors {
		proc.shutdown()
	}
}

// Create a new batch processor
func NewBatchProcessor(interval time.Duration, batchSize int, id int) *BatchProcessor {
	bp := &BatchProcessor{
		updates:       make([]updateMsg, 0, batchSize),
		batchInterval: interval,
		batchSize:     batchSize,
		active:        true,
		id:            id,
		workerPool:    make(chan struct{}, runtime.NumCPU()/2+1),
		pendingWork:   make(chan []updateMsg, 100),
		done:          make(chan struct{}),
	}
	go bp.processLoop()
	return bp
}

// Main processing loop for batch processor
func (bp *BatchProcessor) processLoop() {
	ticker := time.NewTicker(bp.batchInterval)
	defer ticker.Stop()

	for bp.active {
		select {
		case <-ticker.C:
			bp.collectBatch()
		case updates := <-bp.pendingWork:
			bp.processBatch(updates)
		case <-bp.done:
			return
		}
	}
}

// Collect batch of updates
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
		bp.processBatch(updates)
	}
}

// Process a batch of updates
func (bp *BatchProcessor) processBatch(updates []updateMsg) {
	// Consolidate to keep only latest state per checkbox
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

	// Get current clients
	clientsMutex.RLock()
	activeClients := make([]*SafeWebSocket, 0, len(clients))
	for client := range clients {
		activeClients = append(activeClients, client)
	}
	clientsMutex.RUnlock()

	if len(activeClients) == 0 {
		return
	}

	// Process clients in parallel
	var wg sync.WaitGroup
	chunkSize := 50

	for i := 0; i < len(activeClients); i += chunkSize {
		end := i + chunkSize
		if end > len(activeClients) {
			end = len(activeClients)
		}

		wg.Add(1)
		go func(clients []*SafeWebSocket) {
			defer func() {
				wg.Done()
				if r := recover(); r != nil {
					errorLog.Printf("Recovered in processBatch: %v", r)
				}
			}()

			bp.workerPool <- struct{}{}
			defer func() { <-bp.workerPool }()

			var clientsToRemove []*SafeWebSocket

			for _, client := range clients {
				if !sendUpdatesToClient(client, messages) {
					clientsToRemove = append(clientsToRemove, client)
				}
			}

			// Remove failed clients
			for _, client := range clientsToRemove {
				removeClient(client)
			}
		}(activeClients[i:end])
	}

	wg.Wait()
}

// Send updates to a specific client
func sendUpdatesToClient(client *SafeWebSocket, messages [][]byte) bool {
	if client == nil {
		return false
	}

	if !client.mutex.TryLock() {
		return true // Skip for now, don't remove
	}
	defer client.mutex.Unlock()

	// Check if connection is valid
	if client.conn == nil {
		return false
	}

	now := time.Now()
	// Rate limit per client
	if now.Sub(client.lastSend) < 50*time.Millisecond {
		return true
	}
	client.lastSend = now

	// Safe write with timeout
	client.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	defer client.conn.SetWriteDeadline(time.Time{})

	for _, msg := range messages {
		if err := client.conn.WriteMessage(websocket.BinaryMessage, msg); err != nil {
			if isDebugEnabled() {
				errorLog.Printf("Write error for client %d: %v", client.id, err)
			}
			return false
		}
	}

	return true
}

// Add an update to a batch processor
func (bp *BatchProcessor) addUpdate(id int, state bool, sender *SafeWebSocket) {
	bp.mu.Lock()
	bp.updates = append(bp.updates, updateMsg{id: id, state: state, sender: sender})
	bp.mu.Unlock()
}

// Shutdown this batch processor
func (bp *BatchProcessor) shutdown() {
	if bp.active {
		bp.active = false
		close(bp.done)
	}
}

// Logging utilities
func logInfo(format string, v ...interface{}) {
	if infoLog != nil {
		infoLog.Printf(format, v...)
	}
}

func logError(format string, v ...interface{}) {
	errorLog.Printf(format, v...)
}

func isDebugEnabled() bool {
	return infoLog != nil
}

// Load settings from file
func loadSettings() error {
	data, err := os.ReadFile("settings.json")
	if err != nil {
		if os.IsNotExist(err) {
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

	// Initialize bitArray
	bitArray = make([]bool, settings.BoardSize)
	return nil
}

// Send the entire bit array to a client
func sendBitArrayToClient(conn *websocket.Conn) error {
	bytes := make([]byte, (settings.BoardSize+7)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	defer conn.SetWriteDeadline(time.Time{})
	return conn.WriteMessage(websocket.BinaryMessage, bytes)
}

// Broadcast a change to all clients
func broadcastChange(sender *SafeWebSocket, id int, state bool) {
	batchProcessorPool.addUpdate(id, state, sender)
}

// Broadcast the entire board state to all clients
func broadcastToAll() {
	clientsMutex.RLock()
	clientCount := len(clients)
	if clientCount == 0 {
		clientsMutex.RUnlock()
		return
	}
	activeClients := make([]*SafeWebSocket, 0, clientCount)
	for client := range clients {
		activeClients = append(activeClients, client)
	}
	clientsMutex.RUnlock()

	logInfo("Broadcasting full state to %d clients", len(activeClients))

	bytes := make([]byte, (settings.BoardSize+7)/8)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			bytes[i/8] |= 1 << uint(7-(i%8))
		}
	}

	// Process in chunks
	chunkSize := 20
	var clientsToRemove []*SafeWebSocket
	var mutex sync.Mutex

	var wg sync.WaitGroup
	for i := 0; i < len(activeClients); i += chunkSize {
		end := i + chunkSize
		if end > len(activeClients) {
			end = len(activeClients)
		}

		wg.Add(1)
		go func(clients []*SafeWebSocket) {
			defer wg.Done()

			localClients := make([]*SafeWebSocket, 0)
			for _, client := range clients {
				if client == nil || client.conn == nil {
					localClients = append(localClients, client)
					continue
				}

				if !client.mutex.TryLock() {
					continue
				}

				client.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
				err := client.conn.WriteMessage(websocket.BinaryMessage, bytes)
				client.conn.SetWriteDeadline(time.Time{})
				client.mutex.Unlock()

				if err != nil {
					localClients = append(localClients, client)
				}
			}

			// Queue clients for removal
			if len(localClients) > 0 {
				mutex.Lock()
				clientsToRemove = append(clientsToRemove, localClients...)
				mutex.Unlock()
			}
		}(activeClients[i:end])

		// Small pause between starting goroutines
		time.Sleep(10 * time.Millisecond)
	}

	wg.Wait()

	// Remove failed clients
	for _, client := range clientsToRemove {
		removeClient(client)
	}
}

// WebSocket connection handler
func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logError("Error upgrading to WebSocket: %v", err)
		return
	}

	// Create safe connection
	safeConn := &SafeWebSocket{
		conn: conn,
		id:   atomic.AddUint64(&clientIdCounter, 1),
	}

	// Register client
	clientsMutex.Lock()
	clients[safeConn] = true
	clientCount := len(clients)
	clientsMutex.Unlock()
	atomic.AddInt32(&activeConn, 1)

	logInfo("New client %d connected. Total clients: %d", safeConn.id, clientCount)

	defer func() {
		removeClient(safeConn)
		if r := recover(); r != nil {
			errorLog.Printf("Recovered in handleWebSocket: %v", r)
		}
	}()

	// Send board size
	sizeMsg := make([]byte, 5)
	sizeMsg[0] = 1 // type 1 = board size
	sizeMsg[1] = byte(settings.BoardSize >> 24)
	sizeMsg[2] = byte(settings.BoardSize >> 16)
	sizeMsg[3] = byte(settings.BoardSize >> 8)
	sizeMsg[4] = byte(settings.BoardSize)

	safeConn.mutex.Lock()
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	err = conn.WriteMessage(websocket.BinaryMessage, sizeMsg)
	conn.SetWriteDeadline(time.Time{})
	safeConn.mutex.Unlock()

	if err != nil {
		logError("Error sending board size: %v", err)
		return
	}

	// Send initial board state
	safeConn.mutex.Lock()
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	err = sendBitArrayToClient(conn)
	conn.SetWriteDeadline(time.Time{})
	safeConn.mutex.Unlock()

	if err != nil {
		logError("Error sending initial state: %v", err)
		return
	}

	// Message handling loop
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				if isDebugEnabled() {
					logError("WebSocket read error: %v", err)
				}
			}
			break
		}

		if len(message) == 5 && message[0] == 0 {
			id := int(message[1])<<16 | int(message[2])<<8 | int(message[3])
			state := message[4] != 0

			if id >= 0 && id < settings.BoardSize {
				bitArray[id] = state
				if isDebugEnabled() {
					logInfo("Checkbox %d set to %v", id, state)
				}
				broadcastChange(safeConn, id, state)
			}
		}
	}
}

// Remove a client
func removeClient(client *SafeWebSocket) {
	if client == nil {
		return
	}

	clientsMutex.Lock()
	defer clientsMutex.Unlock()

	if _, exists := clients[client]; exists {
		// Safe close
		if client.conn != nil {
			conn := client.conn
			client.conn = nil

			defer func() {
				if r := recover(); r != nil {
					errorLog.Printf("Recovered in connection close: %v", r)
				}
			}()

			conn.SetWriteDeadline(time.Now().Add(time.Second))
			conn.WriteMessage(websocket.CloseMessage, 
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			conn.Close()
		}

		delete(clients, client)
		atomic.AddInt32(&activeConn, -1)
		
		if isDebugEnabled() {
			logInfo("Client %d disconnected. Total clients: %d", client.id, len(clients))
		}
	}
}

// Save board state to disk
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

// Load board state from disk
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

	logInfo("Board state loaded from disk")
}

func main() {
	// Parse flags
	listenAddr := flag.String("addr", "0.0.0.0:1335", "HTTP service address")
	resetBoard := flag.Bool("reset", false, "Reset the board to empty state on startup")
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	batchInterval := flag.Duration("batch-interval", 5*time.Millisecond, "Interval for batching updates")
	batchSize := flag.Int("batch-size", 10000, "Size of each batch")
	numProcessors := flag.Int("processors", runtime.NumCPU(), "Number of batch processors")
	flag.Parse()

	// Setup loggers
	errorLog = log.New(os.Stderr, "ERROR: ", log.Ltime|log.Lshortfile)
	if *verbose {
		infoLog = log.New(os.Stdout, "INFO: ", log.Ltime)
	} else {
		infoLog = log.New(io.Discard, "", 0)
	}

	// Initialize batch processor
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

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup HTTP server with timeouts
	server := &http.Server{
		Addr:              *listenAddr,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Start periodic save and stats reporting
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				saveToDisk()
				conns := atomic.LoadInt32(&activeConn)
				logInfo("Active connections: %d", conns)
				if conns > 0 {
					broadcastToAll()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Handle routes
	http.HandleFunc("/ws", handleWebSocket)
	http.Handle("/", http.FileServer(http.Dir("static")))

	// Start server
	go func() {
		logInfo("Server starting on %s", *listenAddr)
		logInfo("Board size: %d", settings.BoardSize)
		logInfo("Batch interval: %v, Batch size: %d, Processors: %d", 
			*batchInterval, *batchSize, *numProcessors)

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			logError("Server error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logInfo("Shutting down server...")
	cancelCtx, cancelFunc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFunc()

	batchProcessorPool.shutdown()
	if err := server.Shutdown(cancelCtx); err != nil {
		logError("Server shutdown error: %v", err)
	}

	saveToDisk()
	logInfo("Server stopped")
}
