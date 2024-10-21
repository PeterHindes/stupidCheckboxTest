package checkboxserver

import (
	// "encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"math"

	"github.com/gorilla/websocket"
	// "github.com/minio/sha256-simd"

	"github.com/PeterHindes/bitarrayutils/fileutils"
	// "github.com/PeterHindes/bitarrayutils/compression/brle"
	// "github.com/PeterHindes/bitarrayutils/compression/rle"
)

// Message type enums
// we have change, checkinhash, and versioncheck
// which are encoded as 0, 1, and 2 respectively using 4 bits
const (
	Change = 0
	CheckInHash = 1
	VersionCheck = 2
)

// Handle WebSocket connections
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

	// Right after it opens send the full bitArray, the client will simply verify that the blob is the correct length because otherwise it is an rle encoded blob
	err = sendBitArray(conn, bitArray[:])
	if err != nil {
		log.Println("Failed to send bitArray:", err)
		return
	}

	// Send check the channel for changes in the bitArray and send them to the client
	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			// Wait for the update of the encoded bitArray
			<-encodedArrayMutex
			log.Println("Sending encoded bitArray...")
			err := sendBitArray(conn, encodedBitArray)
			if err != nil {
				log.Println("Failed to send bitArray:", err)
				break
			}
			encodedArrayMutex <- true
		}
	}()

	// Read messages from WebSocket, they will all be byte arrays
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Failed to read message from WebSocket:", err)
			break
		}

		// Figure out what type of message we have

		// First convert what we recived to a bitArray
		byteArray := message
		bitArray := make([]bool, len(byteArray)*8)
		for i := 0; i < len(byteArray); i++ {
			for j := 0; j < 8; j++ {
				bitArray[i*8+j] = byteArray[i]&(1<<uint8(7-j)) != 0
			}
		}

		// Check the message type in first 4 bits
		// use a slice to get the first 4 bits
		messageType := bitArray[:4]

		// convert message type to an integer
		// we will use this to determine what to do with the message
		messageTypeInt := 0
		for i := 0; i < 4; i++ {
			if messageType[i] {
				messageTypeInt |= 1 << uint8(3-i)
			}
		}

		log.Println("Message type:", messageType) // TODO make this use the enums to print the actual message type

		// Handle the message
		switch messageTypeInt {
			case Change: handleChange(bitArray[4:]) // Changes will be in 20 bits indicating the position of the change
			case CheckInHash: handleCheckInHash(bitArray[4:]) // CheckInHash will be in 256 bits as a sha256 hash
			case VersionCheck: handleVersionCheck(bitArray[4:]) // VersionCheck will be in 32 bits as an integer
		}
	}

}
func sendBitArray(conn *websocket.Conn, bitArray []bool) error {
	// Encode the bitArray into a binary message without json
	// Calculate the number of bytes needed to store the bit array
	byteArraySize := int(math.Ceil(float64(len(bitArray)) / 8))
	byteArray := make([]byte, byteArraySize)
	for i := 0; i < len(bitArray); i++ {
		if bitArray[i] {
			byteArray[i/8] |= 1 << uint8(7-i%8)
		}
	}

	// Handle the remaining bits TODO: Check if ai did a good job here
	remainingBits := len(byteArray)*8 - len(bitArray)
	if remainingBits > 0 {
		byteArray[len(byteArray)-1] >>= uint8(remainingBits)
		byteArray[len(byteArray)-1] <<= uint8(remainingBits)
	}

	// Send the bitArray
	err := conn.WriteMessage(websocket.BinaryMessage, byteArray)
	if err != nil {
		return err
	}

	return nil
}

func handleChange(bitData []bool) {
	// decode the index from the first 20 bits
	index := 0
	for i := 0; i < 20; i++ {
		if bitData[i] {
			index |= 1 << uint8(19-i)
		}
	}

	// decode the value from the last bit
	value := bitData[20]

	bitArray[index] = value
}

var bitArray [1000000]bool

// sliding window that can hold 5*5 256bit hashes
var validHashes = make([][32]byte, 25)
var validHashesIndex = 0
var statistics = make(map[string]int)
var statisticsMutex = make(chan bool, 1)

func main() {

	// Initialize the bitArray with values from a file
	err := fileutils.loadBinaryFile(&bitArray, "bitArray.bin")
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
	// Also save every 5 seconds
	go func() {
		for {
			time.Sleep(5 * time.Second)
			saveBinaryFile(bitArray[:], "bitArray.bin")
			log.Println("BitArray saved to bitArray.bin")
			// Also save the statistics to a json file
			statisticsMutex <- true
			file, err := os.Create("statistics.json")
			if err != nil {
				log.Println("Failed to create statistics file:", err)
			} else {
				encoder := json.NewEncoder(file)
				err = encoder.Encode(statistics)
				if err != nil {
					log.Println("Failed to encode statistics:", err)
				}
				file.Close()
			}
		}
	}()

	// Every 200 milliseconds, update the encoded bitArray
	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			updateEncodedBitArray()
		}
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
