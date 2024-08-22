// this file will run the change encoding on diff.bin and output ediff.bin

package main

import (
	"fmt"
	"math/rand"
	"math"
	"os"
	"sync"
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



	// Sweep the power of two and the number of changes

	// Create a 2D array to store the sizes
	// make constants for the step size and max for each parameter
	const powerStepSize = 2
	const maxPower = 20
	const initialPower = 2
	const changesStepSize = 1000
	const maxChanges = 1000000

	sizes := make([][]int, maxPower/powerStepSize+1)
	for i := 0; i < maxPower/powerStepSize+1; i++ {
		sizes[i] = make([]int, maxChanges/changesStepSize+1)
	}

	numberOfThreadsThatWillRun := (maxPower/powerStepSize) * (maxChanges/changesStepSize)

	var wg sync.WaitGroup
	const maxThreads = 500
	threadCount := 0
	completedThreads := 0

	for i := initialPower; i <= maxPower; i += powerStepSize {
		for j := 0; j <= maxChanges; j+= changesStepSize {
			wg.Add(1)
			go func(powerOfTwo, changes int) {
				x := (powerOfTwo - initialPower) / powerStepSize
				y := changes / changesStepSize
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
				sizes[x][y] = countRuns(array, powerOfTwo)
				// fmt.Print("r", powerOfTwo, "f", changes, " ")
			}(i, j)

			threadCount++
			if threadCount == maxThreads {
				wg.Wait()
				completedThreads += threadCount
				threadCount = 0
				fmt.Println(float64(completedThreads*100)/float64(numberOfThreadsThatWillRun), "% Complete")
			}
		}
	}

	wg.Wait()

/*
	// find min
	min := arraySize
	minIndex := make([]int, 2)
	for i := 0; i < maxPower/powerStepSize; i+= powerStepSize {
		for j := 0; j < maxChanges/changesStepSize; j+= changesStepSize {
			if sizes[i][j] < min {
				min = sizes[i][j]
				minIndex[0] = i*powerStepSize+initialPower
				minIndex[1] = j*changesStepSize
			}
		}
	}
	fmt.Println("Smallest size of encoded array:", min)
	fmt.Println("Average run length that produced the smallest size:", minIndex[0], "\nChanges that produced the smallest size:", minIndex[1])
*/

	// Save to xls file
	saveToXLS(sizes, initialPower, maxPower, powerStepSize, maxChanges, changesStepSize)


/*
	array := createChanges(arraySize, 0)

	len := countRuns(array, 20)
	
	fmt.Println("Size of encoded array:", len)
*/
}

func createChanges(arraySize, changes int) []bool {
	// Create two boolean arrays
	array1 := make([]bool, arraySize)

	// Fill the arrays with random data
	fillRandomData(array1)

	// create array2 as a copy of array1 with 
	array2 := make([]bool, arraySize)
	copy(array2, array1)
	// Change random bits in array2
	// Make sure that we only change a bit once, every change is unique
	changedBits := make(map[int]bool)
	for i := 0; i < changes; i++ {
		index := rand.Intn(arraySize)
		for changedBits[index] {
			index = rand.Intn(arraySize)
		}
		array2[index] = !array2[index]
		changedBits[index] = true
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

/*
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
*/

	// // print the last bit of the array
	// if array[len(array)-1] {
	// 	fmt.Println("Change")
	// } else {
	// 	fmt.Println("No Change")
	// }

	return len(encoded)

}

/*

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

*/

// Iterative function to split a run into multiple runs
func splitByPowerOfTwo(run, power int, end bool) ([]int, []bool) {
	runs := make([]int, 0)
	ends := make([]bool, 0)

	for run > power {
		runs = append(runs, power-1)
		ends = append(ends, false)
		run -= power - 1
	}

	runs = append(runs, run)
	ends = append(ends, end)

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


func saveToXLS(sizes [][]int, initialPower, maxPower, powerStepSize, maxChanges, changesStepSize int) {
	// Create a new excel file
	f := excelize.NewFile()

	// Create a new sheet
	_, err := f.NewSheet("Sheet1")

	if err != nil {
		fmt.Println("Error creating new sheet:", err)
		return
	}

	// Set the value of the cell
	f.SetCellValue("Sheet1", "A1", "Power Of Two")
	f.SetCellValue("Sheet1", "B1", "Changes")
	f.SetCellValue("Sheet1", "C1", "Encoded Array Size")
	
	// Fill the sheet with the data
	vert := 1
	for i := 0; i < len(sizes); i++ {
		fmt.Println("Power In Sheet ",i)
		for j := 0; j < len(sizes[i]); j++ {
			vert += 1
			encSize := sizes[i][j]
			f.SetCellValue("Sheet1", fmt.Sprintf("A%d", vert), i*powerStepSize+initialPower)
			f.SetCellValue("Sheet1", fmt.Sprintf("B%d", vert), j*changesStepSize)
			f.SetCellValue("Sheet1", fmt.Sprintf("C%d", vert), encSize)
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