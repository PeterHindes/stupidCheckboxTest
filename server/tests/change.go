// this file will run the change encoding on diff.bin and output ediff.bin

package main

import (
	"fmt"
	"math/rand"
	"math"
	"os"
	_"sync"
	"github.com/xuri/excelize/v2"
)

func main() {
	// Set the size of the board to 1 million
	const arraySize = 1000000
	
	// // Specify the path to the binary file
	// filePath := "diff.bin"

	// // Read the binary file
	// data, err := ioutil.ReadFile(filePath)
	// if err != nil {
	// 	fmt.Println("Error reading file:", err)
	// 	return
	// }

	// // convert data to a boolean array
	// arraySize := len(data) * 8
	// array := make([]bool, arraySize)
	// for i := 0; i < arraySize; i++ {
	// 	byteIndex := i / 8
	// 	bitIndex := i % 8
	// 	bit := (data[byteIndex] >> (7 - bitIndex)) & 1
	// 	array[i] = bit == 1
	// }

/*

	// Sweep the average parameter between 1 and 1000000
	// track the sizes of the encoded arrays
	// print the sizes of the encoded arrays
	// print the smallest size and the average that produced it
	sizes := make([][]int, 200)
	for i := 0; i < 200; i++ {
		sizes[i] = make([]int, 500000/50)
		// for j := 0; j < 10000; j++ {
		// 	sizes[i][j] = arraySize
		// }
	}

	var wg sync.WaitGroup
	maxThreads := 2000
	threadCount := 0

	for i := 4; i < 2000; i += 10 {
		j := 0
		wg.Add(1)
		go func(runlen, changes int) {
			x := (runlen - 4) / 10
			y := changes / 50
			defer func() {
				if r := recover(); r != nil {
					fmt.Println()
					// Handle the panic here or ignore it
					fmt.Println("Recovered from panic:", r)
					fmt.Println("X:", x, "Y:", y)
				}
				wg.Done()
			}()
			array := createChanges(arraySize, changes)
			// Print the changes array
			// fmt.Println("Changes array:", array)

			sizes[x][y] = countRuns(array, float64(runlen))
			fmt.Print("r", runlen, "f", changes, " ")
		}(i, j)

		threadCount++
		if threadCount >= maxThreads {
			wg.Wait()
			threadCount = 0
		}
	}

	wg.Wait()
	// find min
	min := arraySize
	minIndex := make([]int, 2)
	for i := 0; i < 200; i++ {
		for j := 0; j < 500000/50; j++ {
			if sizes[i][j] < min {
				min = sizes[i][j]
				minIndex[0] = i*10+4
				minIndex[1] = j*50
			}
		}
	}
	fmt.Println("Smallest size of encoded array:", min)
	fmt.Println("Average run length that produced the smallest size:", minIndex[0], "\nChanges that produced the smallest size:", minIndex[1])

	

	// array := createChanges(arraySize, 61370)
	// size := countRuns(array, 4)
	// fmt.Println("Size of encoded array:", size)

	// Generate the bar graph
	// barGraph(goodSizes, goodSizePositons)

	// Save to xls file
	saveToXLS(sizes)

	*/

	array := createChanges(arraySize, 0)

	len := countRuns(array, 1000000)
	
	fmt.Println("Size of encoded array:", len)
}

func createChanges(arraySize, changes int) []bool {
	// Create two boolean arrays
	array1 := make([]bool, arraySize)
	// array2 := make([]bool, arraySize)

	// Fill the arrays with random data
	fillRandomData(array1)
	// fillRandomData(array2)

	// create array2 as a copy of array1 with 
	array2 := make([]bool, arraySize)
	copy(array2, array1)
	for i := 0; i < changes; i++ {
		array2[rand.Intn(arraySize)] = !array2[rand.Intn(arraySize)]
	}

	// Calculate the differences between the two arrays
	// Create two boolean arrays for the differences
	arraynegdiff := make([]bool, arraySize)
	arrayposdiff := make([]bool, arraySize)
	for i := 0; i < arraySize; i++ {
		arraynegdiff[i] = array1[i] && !array2[i]
		arrayposdiff[i] = !array1[i] && array2[i]
	}

	// Combine the changes into a single array using or
	// Create a new array for the combined differences
	arraydiff := make([]bool, arraySize)
	for i := 0; i < arraySize; i++ {
		arraydiff[i] = arraynegdiff[i] || arrayposdiff[i]
	}

	// Print the last bit of the combined array
	fmt.Print("Array diff: ")
	if arraydiff[arraySize-1] {
		fmt.Println("Change")
	} else {
		fmt.Println("No Change")
	}

	return arraydiff
}



// this encoding will take a boolean array that includes all the flips
// the encoding will be as follows:
// [number of zeros (no flip) as 7 bits][positive or negative change as 1 bit where a zero is no change and a one is a change] repeat until end of array to encode
// this encoding will be done in a new array but the array will not be aligned with the input array and will instead be a series that conforms to the encoding
// First we will record the lengths of runs in an int array
// then we will convert the runs to the encoding
func countRuns(array []bool, powerOfTwo int) int {
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

	// // Print the first 10 runs
	// fmt.Println("Runs:", runs[:10])
	// fmt.Println("Ends:", ends[:10])
	// Print all runs
	fmt.Println("Runs:", runs)
	fmt.Println("Ends:", ends)

	// // Find the average run length
	// total := 0
	// for i := 0; i < len(runs); i++ {
	// 	total += runs[i]
	// }
	// average := float64(total) / float64(len(runs))
	// fmt.Println("Average run length:", average)

	// // Find the nearest power of two to the average
	// lastPower := 1.0
	// power := 2.0
	// for power < average {
	// 	lastPower = power
	// 	power *= 2
	// }
	// if (average - lastPower) < (power - average) {
	// 	power = lastPower
	// }
	// powerInt := int(power)
	// // fmt.Println("Nearest power of two:", powerInt)
	
	// // convert the powerInt to an int representing the power of two
	// powerOfTwo := int(math.Log2(float64(powerInt)))
	// // fmt.Println("Power of two:", powerOfTwo)

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

	// fmt.Println("New Runs:", newRuns[:10])
	// fmt.Println("New Ends:", newEnds[:10])

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

	// Print the array so far for debuging
	fmt.Println("Encoded array so far:", encoded)

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


	fmt.Println("The power of two:", powerOfTwo)
	fmt.Println("The encoded array:")
	for i := 0; i < len(encoded); i++ {

		bodyPos := i - 5
		runPos := bodyPos % (powerOfTwo+1)

		// Put a space after the bits that represent the encoding length
		if bodyPos == 0 {
			fmt.Print("_")
		}
		// Put a space after the bits that represent the encoding length
		if (runPos == 0 && bodyPos > 0) {
			fmt.Print(" ")
		}
		// Put a dash after the bits that represent the runs length before the bit that represents how the run ends
		if (runPos == powerOfTwo && bodyPos > 0) {
			fmt.Print("-")
		}
		
		if encoded[i] {
			fmt.Print("1")
		} else {
			fmt.Print("0")
		}
	}
	fmt.Println("")

	// // print the last bit of the array
	// if array[len(array)-1] {
	// 	fmt.Println("Change")
	// } else {
	// 	fmt.Println("No Change")
	// }

	return len(encoded)

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

// print the changeEncoded runs
// we should print them in this format
// [number of zeros (no flip) as 7 bits][positive or negative change as 1 bit where a zero is no change and a one is a change] repeat until end of array to encode
// but with annotations like
// 5 zeros, no change
// 3 zeros, change
func printCenc(name string, array []bool) {
	// Calculate the length of array
	arraySize := len(array)

	// loop through the input array to extract the data
	for i := 0; i < arraySize; i+=8 {
		run := 0 // max possible run is 127
		// convert the 7 bits to an integer
		// copy 7 bits from the array at the i index
		for j := 0; j < 7; j++ {
			if(array[i+j]) {
				run += int(math.Pow(2, float64(j)))
			}
		}


		// decode the last bit as change or no change
		change := array[i+7]

		// print the run
		fmt.Printf("%d zeros, ", run)
		if change {
			fmt.Println("change")
		} else {
			fmt.Println("no change")
		}

		// Stop after 25 runs
		if i >= 200 {
			break
		}
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


func saveToXLS(sizes [][]int) {
	// Create a new excel file
	f := excelize.NewFile()

	// Create a new sheet
	_, err := f.NewSheet("Sheet1")

	if err != nil {
		fmt.Println("Error creating new sheet:", err)
		return
	}

	// Set the value of the cell
	f.SetCellValue("Sheet1", "A1", "Average Run Length")
	f.SetCellValue("Sheet1", "B1", "Changes")
	f.SetCellValue("Sheet1", "C1", "Encoded Array Size")
	
	// Fill the sheet with the data
	for i := 0; i < 200; i++ {
		for j := 0; j < 500000/50; j++ {
			f.SetCellValue("Sheet1", fmt.Sprintf("A%d", i*500000/50+j+2), i*10+4)
			f.SetCellValue("Sheet1", fmt.Sprintf("B%d", i*500000/50+j+2), j*50)
			f.SetCellValue("Sheet1", fmt.Sprintf("C%d", i*500000/50+j+2), sizes[i][j])
		}
	}

	// Save the file
	if err := f.SaveAs("results.xlsx"); err != nil {
		fmt.Println("Error saving file:", err)
	}
}

// Fill the boolean array with random data
func fillRandomData(array []bool) {
	for i := 0; i < len(array); i++ {
		array[i] = rand.Intn(2) == 1
	}
}