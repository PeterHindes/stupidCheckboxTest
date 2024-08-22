package main

import (
	"fmt"
	"io/ioutil"
)

func main() {
	// Specify the path to the binary file
	filePath := "/path/to/binary/file.bin"

	// Read the binary file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Perform RLE encoding
	encodedData := rleEncode(data)

	// Calculate the average length of a run
	averageLength := calculateAverageLength(encodedData)

	// Print the average length of a run
	fmt.Println("Average length of a run:", averageLength)
}

func rleEncode(data []byte) []byte {
	encodedData := make([]byte, 0)

	// Perform RLE encoding logic
	count := 1
	for i := 1; i < len(data); i++ {
		if data[i] == data[i-1] {
			count++
		} else {
			encodedData = append(encodedData, byte(count), data[i-1])
			count = 1
		}
	}
	encodedData = append(encodedData, byte(count), data[len(data)-1])

	return encodedData
}

func calculateAverageLength(data []byte) float64 {
	// Calculate the average length of a run logic here
	totalLength := 0
	numRuns := 0

	for i := 0; i < len(data); i += 2  {
		runLength := int(data[i])
		totalLength += runLength
		numRuns++
	}

	averageLength := float64(totalLength) / float64(numRuns)
	return averageLength
}