package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"time"
)


func main() {
	// Set the size of the board to 1 million
	const arraySize = 1000000
	// Create two boolean arrays
	array1 := make([]bool, arraySize)
	// array2 := make([]bool, arraySize)

	// Fill the arrays with random data
	fillRandomData(array1)
	// fillRandomData(array2)

	// create array2 as a copy of array1 with 100 thousand changes
	array2 := make([]bool, arraySize)
	copy(array2, array1)
	for i := 0; i < 100000; i++ {
		array2[rand.Intn(arraySize)] = !array2[rand.Intn(arraySize)]
	}


	// Time this section
	// start := time.Now()

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

	// End the timing
	// elapsed := time.Since(start)
	// fmt.Println("Elapsed time:", elapsed)

	encodeddiff := blankRunEncode(arraydiff, 53)

	// Save to binary file
	saveBinaryFile(arraydiff, "diff.bin")
	// saveBinaryFile(encodeddiff, "ediff.bin")

	// // Print the rle encoded arrays
	// printRle("Negative Differences", encodednegdiff)
	// printRle("Positive Differences", encodedposdiff)


	// Turn the results into a png image
	// Create a new image
	img := createImage(arraynegdiff, arrayposdiff, 100, arraySize)
	// Save the image to a file
	saveImage(img, "diff.png")
}

// Import a csv file which contains the best run lengths for each number of changes, the position in the csv file represents the number of changes and needs to be multiplied by the prescan resolution which is currently 50
func importBestRunLengths() []int {
	// open our csv file
	file, err := os.Open("bestRunLengths.csv")
	if err != nil {
		fmt.Println("Error:", err)
		panic(err)
	}
	defer file.Close()

	// Read the values into an int array
	values := make([]int, 0)
	for {
		var value int
		_, err := fmt.Fscanf(file, "%d,", &value)
		if err != nil {
			break
		}
		values = append(values, value)
	}

	// Return the values
	return values
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

func saveTextFile(array []bool, filename string) {
	// Create a new file
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer file.Close()

	// Save the array to the file
	for i := 0; i < len(array); i++ {
		if array[i] {
			file.WriteString("1")
		} else {
			file.WriteString("0")
		}
	}
}

// Fill the boolean array with random data
func fillRandomData(array []bool) {
	for i := 0; i < len(array); i++ {
		array[i] = rand.Intn(2) == 1
	}
}

// Create an image from the two boolean arrays
func createImage(array1, array2 []bool, width, total int) image.Image {
	// Create a new RGBA image
	img := image.NewRGBA(image.Rect(0, 0, width, total/width))

	// Loop through the two arrays
	for i := 0; i < total; i++ {
		// Set the color of the pixel based on the two arrays
		if array1[i] {
			img.Set(i%width, i/width, color.RGBA{255, 0, 0, 255})
		} else if array2[i] {
			img.Set(i%width, i/width, color.RGBA{0, 255, 0, 255})
		} else {
			img.Set(i%width, i/width, color.RGBA{0, 0, 0, 255})
		}
	}

	return img
}

// Save the image to a file
func saveImage(img image.Image, filename string) {
	// Create a new file
	file, err := os.Create(filename)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer file.Close()

	// Save the image to the file
	err = png.Encode(file, img)
	if err != nil {
		fmt.Println("Error:", err)
	}
}

// New encoding function
// this encoding will take a boolean array that includes all the flips
// the encoding will be as follows:
// [number of zeros (no flip) as 7 bits][positive or negative change as 1 bit where a zero is no change and a one is a change] repeat until end of array to encode
// this encoding will be done in a new array but the array will not be aligned with the input array and will instead be a series that conforms to the encoding

// First we will record the lengths of runs in an int array
// then we will convert the runs to the encoding
func blankRunEncode(array []bool, average float64) []bool {
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


	// fmt.Println("Runs:", runs[:10])
	// fmt.Println("Ends:", ends[:10])


	// // Find the average run length
	// total := 0
	// for i := 0; i < len(runs); i++ {
	// 	total += runs[i]
	// }
	// average := float64(total) / float64(len(runs))
	// fmt.Println("Average run length:", average)

	// // Actually lets try making the "average" the max run length
	// // Find the max run length
	// maxRun := 0
	// for i := 0; i < len(runs); i++ {
	// 	if runs[i] > maxRun {
	// 		maxRun = runs[i]
	// 	}
	// }

	// average := float64(maxRun)
	// fmt.Println("Max run length:", average)

	// // Actually lets make the "average" the value where 90% of the runs are shorter
	// // make an array of ints where each index represents a power of two and the value is the number of runs that are that length or shorter
	// // we have 20 buckets because the max run length is 1 million
	// buckets := make([]int, 20)
	// average := 0.0
	// for i := 0; i < len(runs); i++ {
	// 	for j := 0; j < len(buckets); j++ {
	// 		if runs[i] <= int(math.Pow(2, float64(j))) {
	// 			buckets[j]++
	// 		}
	// 	}
	// }
	// // print buckets and what their power of two is
	// for i := 0; i < len(buckets); i++ {
	// 	fmt.Println("Power of two:", i, "Number of runs:", buckets[i])
	// }

	// // find the bucket where 99% of the runs are shorter
	// total := 0
	// for i := 0; i < len(buckets); i++ {
	// 	total += buckets[i]
	// 	if total >= len(runs)*99/100 {
	// 		fmt.Println("99% of runs are shorter than power of two:", i)
	// 		average = math.Pow(2, float64(i))
	// 		break
	// 	}
	// }

	// Actually lets make the "average" the max number a run could be, 1 million
	// average := 1000000.0


	// Find the nearest power of two to the average
	lastPower := 1.0
	power := 2.0
	for power < average {
		lastPower = power
		power *= 2
	}
	if (average - lastPower) < (power - average) {
		power = lastPower
	}
	powerInt := int(power)
	// fmt.Println("Nearest power of two:", powerInt)
	
	// convert the powerInt to an int representing the power of two
	powerOfTwo := int(math.Log2(float64(powerInt)))
	// fmt.Println("Power of two:", powerOfTwo)

	// Split runs longer than nearest power of two into multiple runs (the first of witch must have and end of false and the second stays true)
	// This requires looping through the runs array and checking if the run is longer than the power of two
	// if it is then we will split it into two runs
	newRuns := make([]int, 0)
	newEnds := make([]bool, 0)
	for i := 0; i < len(runs); i++ {
		newestRuns, newestEnds := splitByPowerOfTwo(runs[i], powerInt)
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
	for j := 0; j < 5; j++ {
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

	fmt.Println("f", average, " ")

	return encoded

	// Show the first 25 runs of the encoded array
	// fmt.Println("First 25 bits of the encoded array:")
	// fmt.Println("")
	// for i := 0; i < len(encoded); i++ {
	// 	if i == 5 {
	// 		fmt.Print(" ")
	// 	}
	// 	if ((i-5) % (powerOfTwo+1)) == 0 && i >= 5 {
	// 		fmt.Print(" ")
	// 	}
		
	// 	if encoded[i] {
	// 		fmt.Print("1")
	// 	} else {
	// 		fmt.Print("0")
	// 	}

	// 	if ((i-5-3) % (powerOfTwo+1)) == 0 && i >= 8 {
	// 		fmt.Print(" ")
	// 	}
	// }
}

// Recursive function to split a run into multiple runs
func splitByPowerOfTwo(run int, power int) ([]int, []bool) {
	// Check if the run is smaller than the power of two
	if run <= power {
		return []int{run}, []bool{true}
	}

	// Split the run into two
	runs := make([]int, 0)
	ends := make([]bool, 0)
	runs = append(runs, power-1)
	ends = append(ends, false)
	newRuns, newEnds := splitByPowerOfTwo(run-power+1, power)
	runs = append(runs, newRuns...)
	ends = append(ends, newEnds...)

	return runs, ends
}
