package main

import (
    "fmt"
    "os"
    "strings"
)

func main() {
    // Read the HTML template file
    inputFile := "template.html"
    outputFile := "output.html"
	// remove the old output file if it exists
	if _, err := os.Stat(outputFile); err == nil {
		os.Remove(outputFile)
	}
    content, err := os.ReadFile(inputFile)
    if err != nil {
        fmt.Println("Error reading file:", err)
        return
    }

    // Find the {{checkbox}} marker
    htmlContent := string(content)
    marker := "{{checkbox}}"
    if !strings.Contains(htmlContent, marker) {
        fmt.Println("Marker not found in the HTML file")
        return
    }

    // Generate 2000 checkboxes
    const rowLength = 20
    const rows = 100
    var checkboxes strings.Builder
	for j := 0; j < rows; j++ {
		checkboxes.WriteString(fmt.Sprintf(`<div id="row%d" class="row" >`, j))
		for i := 0; i < rowLength; i++ {
			var inx = 20*j + i
			checkboxes.WriteString(fmt.Sprintf(
				`<input type="checkbox" id="checkbox%d" name="checkbox%d" onclick="handleBox(%d)"></input>
				`,
				inx,inx,inx))
		}
		checkboxes.WriteString("</div>")
    }

    // Replace the marker with the generated checkboxes
    modifiedContent := strings.Replace(htmlContent, marker, checkboxes.String(), 1)

    // Write the modified content back to the HTML file
    err = os.WriteFile(outputFile, []byte(modifiedContent), 0644)
    if err != nil {
        fmt.Println("Error writing file:", err)
        return
    }

    fmt.Println("Successfully inserted", rowLength*rows,"checkboxes into the HTML file using", rows, "rows and", rowLength, "columns")
}