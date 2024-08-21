package main

import (
	"fmt"
	"strconv"
	"github.com/xuri/excelize/v2"
)

func main() {
	// Open the Excel file
	f, err := excelize.OpenFile("sortedresults.xlsx")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Get all the rows from the sheet
	rows, err := f.GetRows("Sheet1")
	if err != nil {
		fmt.Println(err)
		return
	}

	// Create a map to store the minimum values for each unique value in col2
	minValues := make(map[string]int64)
	col1Values := make(map[string]string)

	// Iterate over the rows
	for _, row := range rows {
		// Get the values from col1, col2, and col3
		col1 := row[0]
		col2 := row[1]
		col3, err := strconv.ParseInt(row[2],2,64)
		if err != nil {
			fmt.Println(err)
			continue
		}

		// Check if the current value in col2 already exists in the map
		if val, ok := minValues[col2]; ok {
			// If the current value in col3 is smaller than the stored minimum value, update the map
			if col3 < val {
				minValues[col2] = col3
				col1Values[col2] = col1
			}
		} else {
			// If the current value in col2 doesn't exist in the map, add it with the current value in col3
			minValues[col2] = col3
			col1Values[col2] = col1
		}
	}

	// Print the minimum values for each unique value in col2
	for key, value := range minValues {
		fmt.Printf("Minimum value for %s: %s, %f\n", key, col1Values[key], value)
	}
}