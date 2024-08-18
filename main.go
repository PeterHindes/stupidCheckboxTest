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

    // Generate 10,000 checkbox inputs
    var checkboxes strings.Builder
	for j := 0; j < 100; j++ {
		checkboxes.WriteString(fmt.Sprintf(`<div id="row%d" class="row filled" >`, j))
		for i := 0; i < 20; i++ {
			var inx = 20*j + i
			checkboxes.WriteString(fmt.Sprintf(
				`<input type="checkbox" id="checkbox%d" name="checkbox%d" onclick="handleBox(%d)"></input>
				`,
				inx,inx,inx))
		}
		checkboxes.WriteString("</div>")
    }

	// for j := 0; j < (1000000-100*20)/20; j++ {
	// 	checkboxes.WriteString(fmt.Sprintf(`<div id="row%d" class="row" >`, j))
	// 	checkboxes.WriteString("</div>")
	// }

    // Replace the marker with the generated checkboxes
    modifiedContent := strings.Replace(htmlContent, marker, checkboxes.String(), 1)

    // Write the modified content back to the HTML file
    err = os.WriteFile(outputFile, []byte(modifiedContent), 0644)
    if err != nil {
        fmt.Println("Error writing file:", err)
        return
    }

    fmt.Println("Successfully inserted 10,000 checkboxes into the HTML file")
}