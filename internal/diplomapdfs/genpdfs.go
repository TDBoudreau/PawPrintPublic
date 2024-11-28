package diplomapdfs

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/phpdave11/gofpdf"
	"github.com/phpdave11/gofpdf/contrib/gofpdi"
	"github.com/xuri/excelize/v2"
)

type DiplomaData struct {
	FullName string
	Degree   string
	Major    string
	Honor    string
	Date     time.Time
}

type BatchJob struct {
	Index int
	Data  []DiplomaData
}

type BatchResult struct {
	Index    int
	PDFBytes []byte
}

func (tm *TaskManager) GeneratePdfs(task *Task, filePath string, batchSize int) error {
	// Get the directory of the executable
	// defer close(task.ProgressChan) // Ensure the channel is closed when done
	task.ProgressChan <- ProgressUpdate{Status: "Starting PDF generation", Progress: 60}
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("Failed to get executable path: %v\n", err)
		return err
	}
	ROOT_DIR := filepath.Dir(exePath)

	// Set the path to the Excel file
	// dataPath := filepath.Join(ROOT_DIR, "data", "input", "test_202410.xlsx")

	sheetName := "Output"

	// Load Excel data
	// f, err := excelize.OpenFile(dataPath)
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		// log.Fatalf("Failed to load Excel data from %s: %v", dataPath, err)
		log.Printf("Failed to load Excel data from %s: %v\n", filePath, err)
		return err
	}

	// Read the sheet
	rows, err := f.GetRows(sheetName)
	if err != nil {
		log.Printf("Failed to get rows from sheet %s: %v\n", sheetName, err)
		return err
	}

	if len(rows) < 1 {
		log.Printf("No rows found in sheet %s\n", sheetName)
		return err
	}
	header := rows[0]

	// Map column names to indices
	colIndex := make(map[string]int)
	for i, colName := range header {
		colIndex[colName] = i
	}

	// Parse the Excel data into a slice of DiplomaData
	var diplomaDataList []DiplomaData
	for i, row := range rows[1:] {
		data := DiplomaData{}
		if idx, ok := colIndex["Full Name"]; ok && idx < len(row) {
			data.FullName = row[idx]
		} else {
			log.Printf("Row %d: Missing 'Full Name'", i+2)
			continue
		}
		if idx, ok := colIndex["Degree"]; ok && idx < len(row) {
			data.Degree = row[idx]
		} else {
			log.Printf("Row %d: Missing 'Degree'", i+2)
			continue
		}
		if idx, ok := colIndex["Major"]; ok && idx < len(row) {
			data.Major = row[idx]
		} else {
			log.Printf("Row %d: Missing 'Major'", i+2)
			continue
		}
		if idx, ok := colIndex["Honor"]; ok && idx < len(row) {
			data.Honor = row[idx]
		} else {
			data.Honor = ""
		}
		if idx, ok := colIndex["Date"]; ok && idx < len(row) {
			dateStr := row[idx]
			date, err := parseDate(dateStr)
			if err != nil {
				log.Printf("Row %d: %v", i+2, err)
				continue
			}
			data.Date = date
		} else {
			log.Printf("Row %d: Missing 'Date'", i+2)
			continue
		}
		diplomaDataList = append(diplomaDataList, data)
	}

	// Coordinates for text placement
	yCoordsOriginal := map[string]float64{
		"name":   443,
		"degree": 298,
		"major":  260,
		"honor":  230,
		"date":   193,
	}

	// Path to the template PDF
	templatePath := filepath.Join(ROOT_DIR, "data", "input", "template", "Template_datamerge_notxt.pdf")

	// Define the font directory
	fontDir := filepath.Join(ROOT_DIR, "data", "input", "fonts")

	// Batch size
	// batchSize := 150

	// Divide diplomaDataList into batches
	var batches []BatchJob
	for i := 0; i < len(diplomaDataList); i += batchSize {
		end := i + batchSize
		if end > len(diplomaDataList) {
			end = len(diplomaDataList)
		}
		batch := BatchJob{
			Index: len(batches),
			Data:  diplomaDataList[i:end],
		}
		batches = append(batches, batch)
	}

	// Channels for jobs and results
	jobs := make(chan BatchJob, len(batches))
	results := make(chan BatchResult, len(batches))

	// WaitGroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	// Number of worker goroutines
	numWorkers := runtime.NumCPU() // Or set to a fixed number

	// Start worker goroutines
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go batchWorker(w, &wg, jobs, results, yCoordsOriginal, templatePath, fontDir)
	}

	// Send jobs
	for _, batch := range batches {
		jobs <- batch
	}
	close(jobs)

	// Wait for all workers to finish
	wg.Wait()
	close(results)

	// Collect all the batch PDFs in order
	pdfBuffers := make([][]byte, len(batches))
	for i := 0; i < len(batches); i++ {
		result := <-results
		pdfBuffers[result.Index] = result.PDFBytes
	}

	// Merge batch PDFs
	task.ProgressChan <- ProgressUpdate{Status: "Saving to final pdf", Progress: 80}
	outputPath := filepath.Join("tmp", fmt.Sprintf("%s.pdf", task.ID))
	err = mergePDFs(pdfBuffers, outputPath)
	if err != nil {
		log.Fatalf("Failed to merge PDFs: %v", err)
	}

	// fmt.Printf("All diplomas have been saved to %s\n", outputPath)

	task.ProgressChan <- ProgressUpdate{Status: "PDF generation completed", Progress: 100}
	task.FinishedAt = time.Now()
	close(task.DoneChan)
	return nil
}

// Batch worker function
func batchWorker(id int, wg *sync.WaitGroup, jobs <-chan BatchJob, results chan<- BatchResult, yCoordsOriginal map[string]float64, templatePath, fontDir string) {
	defer wg.Done()
	for batchJob := range jobs {
		pdfBytes, err := generateBatchPDF(batchJob.Data, yCoordsOriginal, templatePath, fontDir)
		if err != nil {
			log.Printf("Worker %d: Error processing batch %d: %v", id, batchJob.Index, err)
			continue
		}
		results <- BatchResult{Index: batchJob.Index, PDFBytes: pdfBytes}
	}
}

// Function to generate a multi-page PDF for a batch and return it as bytes
func generateBatchPDF(batch []DiplomaData, yCoordsOriginal map[string]float64, templatePath, fontDir string) ([]byte, error) {
	// Create a new PDF object with the font directory specified
	pdf := gofpdf.New("L", "pt", "Letter", fontDir)

	// Register the fonts using only the file names
	pdf.AddUTF8Font("OldEnglishBold", "", "EngraversOldEnglish.ttf")
	pdf.AddUTF8Font("TimesNewRoman", "", "TimesNewRoman.ttf")

	for _, data := range batch {
		err := processDiplomaData(pdf, data, yCoordsOriginal, templatePath)
		if err != nil {
			log.Printf("Error processing diploma for %s: %v", data.FullName, err)
			continue
		}
	}

	// Buffer to hold the PDF data
	var buf bytes.Buffer
	err := pdf.Output(&buf)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Function to merge multiple PDFs
func mergePDFs(pdfBuffers [][]byte, outputPath string) error {
	var pdfReaders []string
	tmpDir := os.TempDir()
	tmpFiles := make([]string, 0, len(pdfBuffers))

	// Write each PDF buffer to a temporary file
	for i, buf := range pdfBuffers {
		tmpFile := filepath.Join(tmpDir, fmt.Sprintf("temp_%d.pdf", i))
		err := os.WriteFile(tmpFile, buf, 0644)
		if err != nil {
			return err
		}
		tmpFiles = append(tmpFiles, tmpFile)
		pdfReaders = append(pdfReaders, tmpFile)
	}

	// Merge PDFs using pdfcpu
	err := api.MergeCreateFile(pdfReaders, outputPath, false, nil)
	if err != nil {
		return err
	}

	// Clean up temporary files
	for _, tmpFile := range tmpFiles {
		os.Remove(tmpFile)
	}

	return nil
}

// Function to process each diploma data and generate a PDF page
func processDiplomaData(pdf *gofpdf.Fpdf, data DiplomaData, yCoordsOriginal map[string]float64, templatePath string) error {
	pdf.AddPage()

	// Import the template PDF page
	importer := gofpdi.NewImporter()
	tpl := importer.ImportPage(pdf, templatePath, 1, "/MediaBox")
	importer.UseImportedTemplate(pdf, tpl, 0, 0, 0, 0)

	// Get page dimensions
	_, pageHeight := pdf.GetPageSize()

	// Adjust y-coordinates
	yCoords := map[string]float64{
		"name":   pageHeight - yCoordsOriginal["name"],
		"degree": pageHeight - yCoordsOriginal["degree"],
		"major":  pageHeight - yCoordsOriginal["major"],
		"honor":  pageHeight - yCoordsOriginal["honor"],
		"date":   pageHeight - yCoordsOriginal["date"],
	}

	// Extract information from the data
	nameText := data.FullName
	degreeText := data.Degree
	majorText := data.Major
	honorText := data.Honor
	dateText := data.Date.Format("January 02, 2006")

	// Split the full name into parts to handle suffixes
	nameParts := strings.Fields(nameText)
	mainName := nameText
	suffix := ""
	if len(nameParts) > 1 {
		lastPart := nameParts[len(nameParts)-1]
		if lastPart == "II" || lastPart == "III" || lastPart == "IV" {
			mainName = strings.Join(nameParts[:len(nameParts)-1], " ")
			suffix = lastPart
		}
	}

	// Font size for the main name
	fontSize := 31.0
	mainFont := "OldEnglishBold"
	mainStyle := ""
	suffixFont := "TimesNewRoman"
	suffixStyle := ""

	// Calculate x-coordinates for centering
	mainNameX := centeredX(pdf, mainName, mainFont, mainStyle, fontSize, suffix, suffixFont, suffixStyle, fontSize)

	// Adjust y-coordinate for the name
	nameY := yCoords["name"]

	// Draw the main name
	pdf.SetFont(mainFont, mainStyle, fontSize)
	pdf.Text(mainNameX, nameY, mainName)

	// Draw the suffix if it exists
	if suffix != "" {
		suffixX := mainNameX + pdf.GetStringWidth(mainName) + 5
		pdf.SetFont(suffixFont, suffixStyle, fontSize)
		pdf.Text(suffixX, nameY, suffix)
	}

	// Adjust y-coordinate for degree
	degreeY := yCoords["degree"]
	degreeHeight := drawWrappedText(pdf, degreeText, "OldEnglishBold", "", 30, degreeY)

	// Adjust y-coordinate for major
	majorY := degreeY + degreeHeight + 10
	majorHeight := drawWrappedText(pdf, majorText, "OldEnglishBold", "", 24, majorY)

	// Adjust y-coordinate for honor or date
	var nextY float64
	if honorText != "" {
		honorY := majorY + majorHeight + 5
		honorHeight := drawWrappedText(pdf, honorText, "OldEnglishBold", "", 18, honorY)
		nextY = honorY + honorHeight + 10
	} else {
		nextY = majorY + majorHeight + 10
	}

	// Draw the date
	drawWrappedText(pdf, dateText, "OldEnglishBold", "", 18, nextY)

	return nil
}

// Function to calculate centered x-coordinate
func centeredX(pdf *gofpdf.Fpdf, mainText string, mainFont string, mainStyle string, mainSize float64, suffixText string, suffixFont string, suffixStyle string, suffixSize float64) float64 {
	pdf.SetFont(mainFont, mainStyle, mainSize)
	textWidth := pdf.GetStringWidth(mainText)

	if suffixText != "" {
		pdf.SetFont(suffixFont, suffixStyle, suffixSize)
		suffixTextWidth := pdf.GetStringWidth(suffixText)
		textWidth += suffixTextWidth + 5 // Adding a small space
	}

	pageWidth, _ := pdf.GetPageSize()
	return (pageWidth - textWidth) / 2
}

func drawWrappedText(pdf *gofpdf.Fpdf, text string, font string, style string, size float64, y float64) float64 {
	pageWidth, _ := pdf.GetPageSize()
	maxWidth := pageWidth - 40
	pdf.SetFont(font, style, size)

	// Regular expression to split on spaces, dashes, and hyphens
	re := regexp.MustCompile(`(\s+|-|–)`)
	parts := re.Split(text, -1)
	delimiters := re.FindAllString(text, -1)

	var lines []string
	currentLine := ""
	for i, part := range parts {
		word := part
		if i > 0 && i-1 < len(delimiters) {
			word = delimiters[i-1] + word
		}
		testLine := currentLine + word
		lineWidth := pdf.GetStringWidth(testLine)
		if lineWidth > maxWidth && currentLine != "" {
			lines = append(lines, currentLine)
			currentLine = word
		} else {
			currentLine = testLine
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	totalHeight := float64(len(lines)) * (size + 5)

	for i, line := range lines {
		lineX := centeredX(pdf, line, font, style, size, "", "", "", 0)
		lineY := y + float64(i)*(size+5)
		pdf.Text(lineX, lineY, line)
	}

	return totalHeight
}

// Function to draw wrapped text
func drawMultiCell(pdf *gofpdf.Fpdf, text string, font string, style string, size float64, y float64) float64 {
	pageWidth, _ := pdf.GetPageSize()
	leftMargin, _, rightMargin, _ := pdf.GetMargins()
	maxWidth := pageWidth - leftMargin - rightMargin // Adjust as needed

	pdf.SetFont(font, style, size)
	pdf.SetY(y)
	pdf.SetX(leftMargin)

	// Centered alignment
	align := "C"

	// Pre-process the text to insert '\n' at dashes/hyphens
	processedText := insertNewlinesAtHyphens(text, pdf, font, style, size, maxWidth)

	// Use MultiCell to write the text with automatic wrapping
	pdf.MultiCell(maxWidth, size+5, processedText, "", align, false)

	// Calculate total height
	numLines := pdf.SplitLines([]byte(processedText), maxWidth)
	totalHeight := float64(len(numLines)) * (size + 5)

	return totalHeight
}

func insertNewlinesAtHyphens(text string, pdf *gofpdf.Fpdf, font string, style string, size float64, maxWidth float64) string {
	pdf.SetFont(font, style, size)

	// Regular expression to split on spaces, dashes, and hyphens, while capturing the delimiters
	re := regexp.MustCompile(`(\s+|-|–)`)
	words := re.Split(text, -1)
	delimiters := re.FindAllString(text, -1)

	var lines []string
	var currentLine strings.Builder

	for i := 0; i < len(words); i++ {
		word := words[i]
		delimiter := ""
		if i < len(delimiters) {
			delimiter = delimiters[i]
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(delimiter)
		}
		currentLine.WriteString(word)

		lineWidth := pdf.GetStringWidth(currentLine.String())
		if lineWidth > maxWidth {
			if delimiter == "-" || delimiter == "–" {
				// Insert newline after the hyphen
				currentLine.WriteString("\n")
			} else {
				// Split at the last hyphen or space
				currentLineStr := currentLine.String()
				lastHyphenIndex := strings.LastIndexAny(currentLineStr, "-–")
				if lastHyphenIndex != -1 {
					// Split at the hyphen
					lines = append(lines, currentLineStr[:lastHyphenIndex+1])
					currentLine.Reset()
					currentLine.WriteString(currentLineStr[lastHyphenIndex+1:])
				} else {
					// Split at the last space
					lastSpaceIndex := strings.LastIndex(currentLineStr, " ")
					if lastSpaceIndex != -1 {
						lines = append(lines, currentLineStr[:lastSpaceIndex])
						currentLine.Reset()
						currentLine.WriteString(currentLineStr[lastSpaceIndex+1:])
					} else {
						// Can't split, force break
						lines = append(lines, currentLineStr)
						currentLine.Reset()
					}
				}
			}
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	// Join the lines with '\n'
	processedText := strings.Join(lines, "\n")
	return processedText
}

var dateLayouts = []string{
	"2006-01-02",      // e.g., "2023-12-10"
	"1/2/2006",        // e.g., "12/10/2023"
	"January 2, 2006", // e.g., "December 10, 2023"
	// ... more layouts
}

// Helper function to parse date
func parseDate(dateStr string) (time.Time, error) {
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, dateStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
}
