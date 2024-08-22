package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
	"math"

	"github.com/gorilla/websocket"
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

	// Send blankRunEncode(bitArray) periodically
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

			// Add the change to the statistics
			statisticsMutex <- true
			if data.State {
				statistics["true"]++
			} else {
				statistics["false"]++
			}
			<-statisticsMutex
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

var bitArray [1000000]bool
var encodedBitArray []bool
var encodedArrayMutex = make(chan bool, 1)
var statistics = make(map[string]int)
var statisticsMutex = make(chan bool, 1)

func updateEncodedBitArray() {
	// Lock the mutex
	<-encodedArrayMutex

	// Encode the bitArray
	encodedBitArray = blankRunEncode(bitArray[:], 5)

	// Unlock the mutex
	encodedArrayMutex <- true
}

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

// Custom RLE encoding function
func blankRunEncode(array []bool, powerOfTwo int) []bool {
	// Calculate the length of array
	arraySize := len(array)
	// Create a new array for the runs
	runs := make([]int, 0)
	// Create a new array for the end values
	ends := make([]bool, 0)

	// keep track of our current run
	run := 0
	// loop through the input array to find zeros
	for i := 0; i < arraySize; i++ {
		if (array[i] == false) {
			run++
		} else {
			runs = append(runs, run)
			run = 0
			ends = append(ends, array[i])
		}
	}
	// Handle the case where the last run is a zero run
	if run > 0 {
		runs = append(runs, run-1)
		ends = append(ends, false)
	}

	powerInt := int(math.Pow(2, float64(powerOfTwo)))

	// Split runs longer than nearest power of two into multiple runs (the first of witch must have and end of false and the second stays true)
	// This requires looping through the runs array and checking if the run is longer than the power of two
	// if it is then we will split it into two runs
	newRuns := make([]int, 0)
	newEnds := make([]bool, 0)
	for i := 0; i < len(runs); i++ {
		newestRuns, newestEnds := splitByPowerOfTwo(runs[i], powerInt, ends[i])
		newRuns = append(newRuns, newestRuns...)
		newEnds = append(newEnds, newestEnds...)
	}


	// From below just encodes using the powerOfTwo and the newRuns and newEnds arrays

	// Create a new array for the change encoding
	// first 5 bits used to store the power of two we are using for max run length
	// then the data payload
	// the data payload consists of a number represented by (power of two) bits followed by a bit to indicate of that run ends with a true or false
	// the data payload is repeated until the end of the array that it represents
	encoded := make([]bool, 0)

	// convert the power of two to a boolean array and append them to the encoded array
	// 5 bits
	for j := 4; j >= 0; j-- {
		encoded = append(encoded, (powerOfTwo>>j)&1 == 1)
	}

	// loop through the alligned newRuns and newEnds arrays to encode the data
	for i := 0; i < len(newRuns); i++ {
	// for i := 0; i < 3; i++ {
		// convert the run to a boolean array and append them to the encoded array
		for j := powerOfTwo - 1; j >= 0; j-- {
			encoded = append(encoded, (newRuns[i]>>j)&1 == 1)
		}

		// then insert the current value of the array
		encoded = append(encoded, newEnds[i])
	}

	// Print the size diffrence between the two arrays
	// fmt.Println()
	// fmt.Println("Original array size:", arraySize)
	// fmt.Println("Encoded array size:  ", len(encoded))


	

	return encoded

}

// Recursive function to split a run into multiple runs
func splitByPowerOfTwo(run, power int, end bool) ([]int, []bool) {
	// Check if the run is smaller than the power of two
	if run <= power {
		return []int{run}, []bool{end}
	}

	// Split the run into two
	runs := make([]int, 0)
	ends := make([]bool, 0)
	runs = append(runs, power-1)
	ends = append(ends, false)
	newRuns, newEnds := splitByPowerOfTwo(run-power+1, power, end)
	runs = append(runs, newRuns...)
	ends = append(ends, newEnds...)

	return runs, ends
}