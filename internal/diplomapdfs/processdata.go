package diplomapdfs

import (
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/xuri/excelize/v2"
)

type DegreeData struct {
	Term     int
	FullName string
	Degree   string
	Major    string
	Honor    string
}

type TermLookup struct {
	Name     string
	Code     int
	DateText string
}

type DegreeLookup struct {
	Code     string
	Text     string
	CodeType string
}

type LookupMaps struct {
	TermLookupMap   map[int]TermLookup
	DegreeLookupMap map[string]DegreeLookup
}

type GraduateDegree struct {
	FullName string
	Degree   string
	Major    string
	Honor    string
	Date     string
}

func (tm *TaskManager) ProcessData(task *Task, filePath string) error {
	// Simulate processing steps
	task.ProgressChan <- ProgressUpdate{Status: "Opening Excel file", Progress: 10}
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		log.Println(err)
		return err
	}

	task.ProgressChan <- ProgressUpdate{Status: "Reading rows", Progress: 20}

	termLookupSlice, err := readTermLookup(f)
	if err != nil {
		log.Println(err)
		return err
	}

	degreeLookupSlice, err := readDegreeLookup(f)
	if err != nil {
		log.Println(err)
		return err
	}

	degreeDataSlice, err := readDegreeData(f)
	if err != nil {
		log.Println(err)
		return err
	}

	graduateData := make([]GraduateDegree, 0, len(degreeDataSlice))
	lookupMaps := LookupMaps{
		TermLookupMap:   make(map[int]TermLookup, len(termLookupSlice)),
		DegreeLookupMap: make(map[string]DegreeLookup, len(degreeLookupSlice)),
	}

	for _, term := range termLookupSlice {
		lookupMaps.TermLookupMap[term.Code] = term
	}

	for _, degree := range degreeLookupSlice {
		lookupMaps.DegreeLookupMap[degree.Code] = degree
	}

	task.ProgressChan <- ProgressUpdate{Status: "Processing data", Progress: 30}
	for _, graduate := range degreeDataSlice {
		term := lookupMaps.TermLookupMap[graduate.Term]
		degree := lookupMaps.DegreeLookupMap[graduate.Degree]
		major := lookupMaps.DegreeLookupMap[graduate.Major]
		honor := lookupMaps.DegreeLookupMap[graduate.Honor]

		output := GraduateDegree{
			FullName: graduate.FullName,
			Degree:   degree.Text,
			Major:    major.Text,
			Honor:    honor.Text,
			Date:     term.DateText,
		}

		graduateData = append(graduateData, output)
	}

	// Writing to a new sheet
	index, err := f.NewSheet("Output")
	if err != nil {
		log.Println(err)
		return err
	}

	dataToWrite := [][]string{}
	for i, grad := range graduateData {
		rowIndex := i + 2 // start at second row, first row will have headers

		// Write headers
		if i == 0 {
			dataToWrite = append(dataToWrite, []string{"Full Name", "Degree", "Major", "Honor", "Graduation Date"})
			f.SetCellValue("Output", "A1", "Full Name")
			f.SetCellValue("Output", "B1", "Degree")
			f.SetCellValue("Output", "C1", "Major")
			f.SetCellValue("Output", "D1", "Honor")
			f.SetCellValue("Output", "E1", "Date")
		}

		// Collect row data for resizing columns
		row := []string{
			grad.FullName,
			grad.Degree,
			grad.Major,
			grad.Honor,
			grad.Date,
		}
		dataToWrite = append(dataToWrite, row)

		// Write data to sheet
		f.SetCellValue("Output", fmt.Sprintf("A%d", rowIndex), grad.FullName)
		f.SetCellValue("Output", fmt.Sprintf("B%d", rowIndex), grad.Degree)
		f.SetCellValue("Output", fmt.Sprintf("C%d", rowIndex), grad.Major)
		f.SetCellValue("Output", fmt.Sprintf("D%d", rowIndex), grad.Honor)
		f.SetCellValue("Output", fmt.Sprintf("E%d", rowIndex), grad.Date)
	}

	// Adjust column widths based on data
	adjustColumnWidths(f, "Output", dataToWrite)

	// Set "Output" as the active sheet and save the file
	f.SetActiveSheet(index)
	if err := f.Save(); err != nil {
		log.Printf("Failed to save file: %v\n", err)
		return err
	}

	task.ProgressChan <- ProgressUpdate{Status: "Data processing completed", Progress: 50}
	return nil
}

func readTermLookup(f *excelize.File) ([]TermLookup, error) {
	rows, err := f.GetRows("Term & Date Lookup")
	if err != nil {
		return []TermLookup{}, err
	}

	termLookupSlice := make([]TermLookup, 0)

	for i, row := range rows {
		if i == 0 {
			// Skip header row
			continue
		}

		code, _ := strconv.Atoi(row[1])
		term := TermLookup{
			Name:     row[0],
			Code:     code,
			DateText: row[2],
		}
		termLookupSlice = append(termLookupSlice, term)
	}

	return termLookupSlice, nil
}

func readDegreeLookup(f *excelize.File) ([]DegreeLookup, error) {
	rows, err := f.GetRows("Degree & Major Lookup")
	if err != nil {
		return []DegreeLookup{}, err
	}

	degreeLookupSlice := make([]DegreeLookup, 0)

	for i, row := range rows {
		if i == 0 {
			// Skip header row
			continue
		}

		degree := DegreeLookup{
			Code:     row[0],
			Text:     row[1],
			CodeType: row[2],
		}
		degreeLookupSlice = append(degreeLookupSlice, degree)
	}

	return degreeLookupSlice, nil
}

func readDegreeData(f *excelize.File) ([]DegreeData, error) {
	rows, err := f.GetRows("Raw Data")
	if err != nil {
		return []DegreeData{}, err
	}

	degreeDataSlice := make([]DegreeData, 0)
	for i, row := range rows {
		if i == 0 {
			// Skip header row
			continue
		}

		term, _ := strconv.Atoi(row[1])
		degreeData := DegreeData{
			Term:     term,
			FullName: row[6],
			Degree:   row[7],
			Major:    row[8],
			Honor:    row[9],
		}

		degreeDataSlice = append(degreeDataSlice, degreeData)
	}

	return degreeDataSlice, nil
}

func adjustColumnWidths(f *excelize.File, sheetName string, data [][]string) error {
	if len(data) == 0 {
		return errors.New("no data")
	}

	colWidths := make([]int, len(data[0]))
	for _, row := range data {
		for j, cell := range row {
			if len(cell) > colWidths[j] {
				colWidths[j] = len(cell)
			}
		}
	}

	colNames := []string{"A", "B", "C", "D", "E"} // Adjust based on your number of columns
	for i, width := range colWidths {
		// Set column width slightly larger than the max content width
		err := f.SetColWidth(sheetName, colNames[i], colNames[i], float64(width)*1.2)
		if err != nil {
			return err
		}
	}

	return nil
}
