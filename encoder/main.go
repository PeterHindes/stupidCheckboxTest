package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	_"time"
	"math"
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

	// Encode the differences using RLE with a max run length of 2^6
	encodeddiff := blankRunEncode(arraydiff, 6)

	// Save to binary file
	saveBinaryFile(arraydiff, "diff.bin")
	saveBinaryFile(encodeddiff, "ediff.bin")

/*
	// Turn the results into a png image
	// Create a new image
	img := createImage(arraynegdiff, arrayposdiff, 100, arraySize)
	// Save the image to a file
	saveImage(img, "diff.png")
*/
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

// Print the RLE encoded array
func printRle(array []bool) {
	// Calculate the power of two
	powerOfTwo := 0
	for i := 4; i >= 0; i-- {
		if array[i] {
			powerOfTwo += 1 << uint(i)
		}
	}

	fmt.Println("The power of two:", powerOfTwo)
	fmt.Println("The encoded array:")
	for i := 0; i < len(array); i++ {

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
		
		if array[i] {
			fmt.Print("1")
		} else {
			fmt.Print("0")
		}
	}
	fmt.Println("")

}