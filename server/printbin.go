package main

import (
	"fmt"
	"io/ioutil"
	"os"
)

func main() {
	// get the bin file from args
	filePath := os.Args[1]

	// Read the entire file
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file:", err)
		return
	}

	// Print the first n bits via argument
	n := os.Args[2]
	split := os.Args[3]
	// convert n to int
	nInt := 0
	fmt.Sscanf(n, "%d", &nInt)
	// convert split to int
	splitInt := 0
	fmt.Sscanf(split, "%d", &splitInt)

	for i := 0; i < nInt; i++ {
		if i >= len(data)*8 {
			break
		}
		byteIndex := i / 8
		bitIndex := i % 8
		bit := (data[byteIndex] >> (7 - bitIndex)) & 1

		if i % (splitInt) == 0 {
			fmt.Print(" ")
			fmt.Print(bit)
		} else {
			fmt.Print(bit)
		}
	}
	fmt.Println()
}