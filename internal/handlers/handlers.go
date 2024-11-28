package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"pawprintpublic/internal/config"
	"pawprintpublic/internal/diplomapdfs"
	"pawprintpublic/internal/driver"
	"pawprintpublic/internal/forms"
	"pawprintpublic/internal/helpers"
	"pawprintpublic/internal/models"
	"pawprintpublic/internal/render"
	"pawprintpublic/internal/repository"
	"pawprintpublic/internal/repository/dbrepo"
	"time"

	"github.com/go-chi/chi"
	"github.com/google/uuid"
)

// Repo the repository used by the handlers
var Repo *Repository

// Repository is the repository type
type Repository struct {
	App *config.AppConfig
	DB  repository.DatabaseRepo
}

// NewRepo creates a new repository
func NewRepo(a *config.AppConfig, db *driver.DB) *Repository {
	return &Repository{
		App: a,
		DB:  dbrepo.NewPostgresRepo(db.SQL, a),
	}
}

// NewTestRepo creates a new repository
func NewTestRepo(a *config.AppConfig) *Repository {
	return &Repository{
		App: a,
		DB:  dbrepo.NewTestingsRepo(a),
	}
}

// NewHandlers sets the repository for the handlers
func NewHandlers(r *Repository) {
	Repo = r
}

func (m *Repository) AdminDashboard(w http.ResponseWriter, r *http.Request) {
	render.Template(w, r, "admin-dashboard.page.tmpl", &models.TemplateData{})
}

// Login shows the login page
func (m *Repository) Login(w http.ResponseWriter, r *http.Request) {
	render.Template(w, r, "login.page.tmpl", &models.TemplateData{
		Form: forms.New(nil),
	})
}

// PostLogin handles logging the user in
func (m *Repository) PostLogin(w http.ResponseWriter, r *http.Request) {
	_ = m.App.Session.RenewToken(r.Context())

	err := r.ParseForm()
	if err != nil {
		m.App.Session.Put(r.Context(), "error", "Error parsing form")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	email := r.Form.Get("email")
	password := r.Form.Get("password")

	form := forms.New(r.PostForm)
	form.Required("email", "password")
	form.IsEmail("email")

	if !form.Valid() {
		data := make(map[string]interface{})
		data["email"] = email

		render.Template(w, r, "login.page.tmpl", &models.TemplateData{
			Form: form,
			Data: data,
		})
		return
	}

	id, _, accessLevel, err := m.DB.Authenticate(email, password)
	if err != nil {
		m.App.Session.Put(r.Context(), "error", "Invalid login credentials")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	m.App.Session.Put(r.Context(), "user_id", id)
	m.App.Session.Put(r.Context(), "access_level", accessLevel)
	m.App.Session.Put(r.Context(), "flash", "Logged in successfully")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// Logout logs a user out
func (m *Repository) Logout(w http.ResponseWriter, r *http.Request) {
	_ = m.App.Session.Destroy(r.Context())
	_ = m.App.Session.RenewToken(r.Context())

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Home is the home page handler
func (m *Repository) Home(w http.ResponseWriter, r *http.Request) {
	render.Template(w, r, "home.page.tmpl", &models.TemplateData{})
}

// FileUploadPage is the upload page handler
func (m *Repository) FileUploadPage(w http.ResponseWriter, r *http.Request) {
	render.Template(w, r, "upload.page.tmpl", &models.TemplateData{})
}

func (m *Repository) UploadHandler(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form data
	err := r.ParseMultipartForm(10 << 20) // 10 MB
	if err != nil {
		m.App.ErrorLog.Println("Error parsing form data")
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}

	// Retrieve the file from form data
	file, handler, err := r.FormFile("file")
	if err != nil {
		m.App.ErrorLog.Println("Error parsing form data")
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	if !helpers.IsValidExcelFile(handler.Filename) {
		m.App.ErrorLog.Println("Error parsing form data")
		http.Error(w, "Invalid file type. Please upload an Excel file.", http.StatusBadRequest)
		return
	}

	// Read the file data into memory
	fileData, err := io.ReadAll(file)
	if err != nil {
		m.App.ErrorLog.Println("Error parsing form data")
		http.Error(w, "Unable to read file", http.StatusInternalServerError)
		return
	}

	// Get the session ID
	sessionID := m.App.Session.Token(r.Context())

	// Generate a unique task ID
	taskID := uuid.New().String()
	fileName := fmt.Sprintf("%s.xlsx", taskID)

	// Store the XLSX file in the database
	err = m.DB.InsertFile(taskID, sessionID, fileName, "xlsx", fileData)
	if err != nil {
		m.App.ErrorLog.Println("Error parsing form data")
		http.Error(w, "Unable to save file", http.StatusInternalServerError)
		return
	}

	// Create a new task
	task := m.App.TaskManager.CreateTask(taskID)

	// Start the processing function in a Goroutine
	go func() {
		defer close(task.ProgressChan)

		err := m.processFileFromDB(task, sessionID)
		if err != nil {
			// Send error update
			task.ProgressChan <- diplomapdfs.ProgressUpdate{Status: "Error", Error: err.Error()}
		}
	}()

	// Return the task ID to the client
	response := map[string]string{"task_id": taskID}
	json.NewEncoder(w).Encode(response)
}

func (m *Repository) processFileFromDB(task *diplomapdfs.Task, sessionID string) error {
	// Retrieve the XLSX file data from the database
	xlsxData, err := m.DB.GetFile(task.ID, "xlsx")
	if err != nil {
		return err
	}

	// Write the XLSX data to a temporary file
	tmpXlsxFilePath := fmt.Sprintf("./tmp/%s.xlsx", task.ID)
	err = os.WriteFile(tmpXlsxFilePath, xlsxData, 0644)
	if err != nil {
		return err
	}
	defer os.Remove(tmpXlsxFilePath)

	// Proceed with processing
	err = m.App.TaskManager.ProcessData(task, tmpXlsxFilePath)
	if err != nil {
		return err
	}

	// Generate PDFs
	err = m.App.TaskManager.GeneratePdfs(task, tmpXlsxFilePath, 100)
	if err != nil {
		return err
	}

	// Read the generated PDF file into memory
	pdfFilePath := fmt.Sprintf("./tmp/%s.pdf", task.ID)
	pdfData, err := os.ReadFile(pdfFilePath)
	if err != nil {
		return err
	}

	// Store the PDF file in the database
	err = m.DB.InsertFile(task.ID, sessionID, fmt.Sprintf("%s.pdf", task.ID), "pdf", pdfData)
	if err != nil {
		return err
	}

	// Optionally delete the PDF file from disk
	err = os.Remove(pdfFilePath)
	if err != nil {
		m.App.ErrorLog.Println("Error removing PDF file from disk:", err)
	}

	return nil
}

// TermSelectPage is the term select handler
func (m *Repository) TermSelectPage(w http.ResponseWriter, r *http.Request) {
	// TODO - Get dynamic list of terms
	terms := [3]string{"2024 Fall Semester", "2025 Spring Semester", "2025 Summer Semester"}

	// Add terms to TemplateData
	data := make(map[string]interface{})
	data["terms"] = terms

	render.Template(w, r, "terms.page.tmpl", &models.TemplateData{
		Data: data,
	})
}

func (m *Repository) SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		m.App.ErrorLog.Println("flush is NOT ok")
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		m.App.ErrorLog.Println("task_id is required")
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Retrieve the task
	task, err := m.App.TaskManager.GetTask(taskID)
	if err != nil {
		m.App.ErrorLog.Println("Task not found")
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*") // Adjust as needed

	// Send the retry directive
	fmt.Fprintf(w, "retry: 0\n\n")
	flusher.Flush()

	// Listen to the progress channel and send updates
	for {
		select {
		case update, ok := <-task.ProgressChan:
			if !ok {
				// Channel closed, task completed
				// Send final event
				fmt.Fprintf(w, "event: done\ndata: Task completed\n\n")
				flusher.Flush()
				return
			}
			data, err := json.Marshal(update)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			// Client disconnected
			return
		}
	}
}

func (m *Repository) StartCleanupJob() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			err := m.DB.DeleteOldFiles(24 * time.Hour)
			if err != nil {
				m.App.ErrorLog.Println("Error cleaning up old files:", err)
			}
		}
	}()
}

func (m *Repository) DownloadHandler(w http.ResponseWriter, r *http.Request) {
	src := chi.URLParam(r, "src")
	if src != "pdf" && src != "xlsx" {
		http.Error(w, "Incorrect src", http.StatusBadRequest)
		return
	}

	taskID := r.URL.Query().Get("task_id")
	if taskID == "" {
		http.Error(w, "task_id is required", http.StatusBadRequest)
		return
	}

	// Retrieve the file from the database
	fileData, err := m.DB.GetFile(taskID, src)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	contentType := "application/octet-stream"
	if src == "pdf" {
		contentType = "application/pdf"
	} else if src == "xlsx" {
		contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.%s\"", taskID, src))
	w.Write(fileData)

	// Optionally, clean up the task and files
	// m.App.TaskManager.DeleteTask(taskID)
	// err = m.DB.DeleteFilesByTask(taskID)
	// if err != nil {
	//     m.App.ErrorLog.Println("Error deleting files for task:", err)
	// }
}

func (m *Repository) AdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := m.DB.AllUsers()
	if err != nil {
		helpers.ServerError(w, errors.New("error fetching users"))
	}

	data := make(map[string]interface{})
	data["users"] = users
	data["usersLength"] = len(users)

	render.Template(w, r, "admin-users.page.tmpl", &models.TemplateData{
		Data: data,
	})
}

func (m *Repository) AdminEditUser(w http.ResponseWriter, r *http.Request) {
	var user models.User
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&user); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Perform server-side validation
	if user.FirstName == "" || user.LastName == "" || user.Email == "" || user.AccessLevel == 0 {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	// Validate email format (you can use regex or a package)
	if !helpers.IsValidEmail(user.Email) {
		http.Error(w, "Invalid email format", http.StatusBadRequest)
		return
	}

	// Update the user in the database
	// Assume updateUser is a function that updates the user and returns an error if any
	err := m.DB.UpdateUser(user)
	if err != nil {
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	// Respond with success
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "User updated successfully"})
}

func (m *Repository) AdminAddUser(w http.ResponseWriter, r *http.Request) {

}

// func (m *Repository) DownloadHandler(w http.ResponseWriter, r *http.Request) {
// 	src := chi.URLParam(r, "src")
// 	if src != "pdf" && src != "xlsx" {
// 		http.Error(w, "incorrect src", http.StatusBadRequest)
// 		return
// 	}

// 	taskID := r.URL.Query().Get("task_id")
// 	if taskID == "" {
// 		http.Error(w, "task_id is required", http.StatusBadRequest)
// 		return
// 	}

// 	// Retrieve the task
// 	task, err := m.App.TaskManager.GetTask(taskID)
// 	if err != nil {
// 		http.Error(w, "Task not found", http.StatusNotFound)
// 		return
// 	}

// 	// Check if the task is completed
// 	select {
// 	case <-task.DoneChan:
// 		// Task completed
// 	default:
// 		http.Error(w, "Task not completed yet", http.StatusAccepted)
// 		return
// 	}

// 	var filePath string

// 	fileName := fmt.Sprintf("%s.%s", taskID, src)
// 	if src == "pdf" {
// 		filePath = fmt.Sprintf("./data/output/pdf/%s", fileName)
// 	} else {
// 		filePath = fmt.Sprintf("./data/input/data/%s", fileName)
// 	}

// 	// Serve the file
// 	if _, err := os.Stat(filePath); os.IsNotExist(err) {
// 		http.Error(w, "File not found", http.StatusNotFound)
// 		return
// 	}

// 	w.Header().Set("Content-Type", fmt.Sprintf("application/%s", src))
// 	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.%s\"", taskID, src))
// 	http.ServeFile(w, r, filePath)

// 	// Optionally, clean up the task
// 	m.App.TaskManager.DeleteTask(taskID)
// }

// func (m *Repository) StartDataProcessingTask(w http.ResponseWriter, r *http.Request) {
// 	err := r.ParseForm()
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't parse form!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	file, handler, err := r.FormFile("file")
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't read uploaded file!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}
// 	defer file.Close()

// 	if !helpers.IsValidExcelFile(handler.Filename) {
// 		m.App.Session.Put(r.Context(), "error", "Invalid file type. Please upload an Excel file.")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	// Process or save the file as needed
// 	// For example, save it to a temporary location and pass the path to the task
// 	filePath := fmt.Sprintf("%s/data/input/data/test_202410.xlsx", ".")
// 	dir := filepath.Dir(filePath)
// 	err = os.MkdirAll(dir, os.ModePerm)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "Failed to create directory for file storage")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	dst, err := os.Create(filePath)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "Error creating file")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}
// 	defer dst.Close()

// 	_, err = io.Copy(dst, file)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "Error saving file")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	// Create a new task
// 	taskID := uuid.New().String()
// 	task := &diplomapdfs.Task{
// 		ID:        taskID,
// 		Status:    diplomapdfs.StatusPending,
// 		CreatedAt: time.Now(),
// 		UpdatedAt: time.Now(),
// 		Type:      "data", // Or "pdf", depending on the task
// 		// Store the cancel function if you want to enable task cancellation
// 	}
// 	m.App.TaskManager.AddTask(task)

// 	// Start processing the task asynchronously
// 	go m.App.TaskManager.ProcessTask(task)

// 	// Return the task ID to the user
// 	response := map[string]string{"task_id": taskID}
// 	json.NewEncoder(w).Encode(response)
// }

// func (m *Repository) StartPdfGenerationTask(w http.ResponseWriter, r *http.Request) {
// 	taskID := uuid.New().String()
// 	task := &diplomapdfs.Task{
// 		ID:        taskID,
// 		Type:      "pdf",
// 		Status:    diplomapdfs.StatusPending,
// 		CreatedAt: time.Now(),
// 		UpdatedAt: time.Now(),
// 	}

// 	m.App.TaskManager.AddTask(task)

// 	go m.App.TaskManager.ProcessTask(task)

// 	// Return the task ID to the user
// 	response := map[string]string{"task_id": taskID}
// 	json.NewEncoder(w).Encode(response)
// }

// // Handler to get task status
// func (m *Repository) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
// 	taskID := r.URL.Query().Get("task_id")
// 	if taskID == "" {
// 		m.App.Session.Put(r.Context(), "error", "task_id is required")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	task, exists := m.App.TaskManager.GetTask(taskID)
// 	if !exists {
// 		m.App.Session.Put(r.Context(), "error", "task not found")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	response := map[string]interface{}{
// 		"task_id": task.ID,
// 		"status":  task.Status,
// 		"error":   task.Error,
// 	}
// 	json.NewEncoder(w).Encode(response)
// }

// // Handler to get task result
// func (m *Repository) GetTaskResult(w http.ResponseWriter, r *http.Request) {
// 	taskID := r.URL.Query().Get("task_id")
// 	if taskID == "" {
// 		m.App.Session.Put(r.Context(), "error", "task_id is required")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	task, exists := m.App.TaskManager.GetTask(taskID)
// 	if !exists {
// 		m.App.Session.Put(r.Context(), "error", "task not found")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	if task.Status != diplomapdfs.StatusCompleted {
// 		m.App.Session.Put(r.Context(), "error", "task not completed yet")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	// Serve the generated PDF file
// 	http.ServeFile(w, r, task.Result)
// }

// func (m *Repository) CancelTask(w http.ResponseWriter, r *http.Request) {
// 	taskID := r.URL.Query().Get("task_id")
// 	if taskID == "" {
// 		m.App.Session.Put(r.Context(), "error", "task_id is required")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	task, exists := m.App.TaskManager.GetTask(taskID)
// 	if !exists {
// 		m.App.Session.Put(r.Context(), "error", "task not found")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	if task.Cancel != nil {
// 		task.Cancel()
// 		// Update task status
// 		task.Status = diplomapdfs.StatusFailed
// 		task.Error = errors.New("task was cancelled by user")
// 		task.UpdatedAt = time.Now()
// 		m.App.TaskManager.UpdateTask(task)
// 	}

// 	response := map[string]string{
// 		"message": "Task cancellation initiated",
// 	}
// 	json.NewEncoder(w).Encode(response)
// }

// // Reservation renders the make a reservation page and displays form
// func (m *Repository) Reservation(w http.ResponseWriter, r *http.Request) {
// 	res, ok := m.App.Session.Get(r.Context(), "reservation").(models.Reservation)
// 	if !ok {
// 		m.App.Session.Put(r.Context(), "error", "can't get reservation from session")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	room, err := m.DB.GetRoomByID(res.RoomID)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't find room!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	res.Room.RoomName = room.RoomName

// 	m.App.Session.Put(r.Context(), "reservation", res)

// 	sd := res.StartDate.Format("2006-01-02")
// 	ed := res.EndDate.Format("2006-01-02")

// 	stringMap := make(map[string]string)
// 	stringMap["start_date"] = sd
// 	stringMap["end_date"] = ed

// 	data := make(map[string]interface{})
// 	data["reservation"] = res

// 	render.Template(w, r, "make-reservation.page.tmpl", &models.TemplateData{
// 		Form:      forms.New(nil),
// 		Data:      data,
// 		StringMap: stringMap,
// 	})
// }

// // PostReservation handles the posting of a reservation form
// func (m *Repository) PostReservation(w http.ResponseWriter, r *http.Request) {
// 	err := r.ParseForm()
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't parse form!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	sd := r.Form.Get("start_date")
// 	ed := r.Form.Get("end_date")

// 	// 2020-01-01 -- 01/02 03:04:05PM '06 -0700

// 	layout := "2006-01-02"

// 	startDate, err := time.Parse(layout, sd)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't parse start date")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	endDate, err := time.Parse(layout, ed)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't get parse end date")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	roomID, err := strconv.Atoi(r.Form.Get("room_id"))
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "invalid data!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	// add this to fix invalid data error
// 	room, err := m.DB.GetRoomByID(roomID)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't find room!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	reservation := models.Reservation{
// 		FirstName: r.Form.Get("first_name"),
// 		LastName:  r.Form.Get("last_name"),
// 		Phone:     r.Form.Get("phone"),
// 		Email:     r.Form.Get("email"),
// 		StartDate: startDate,
// 		EndDate:   endDate,
// 		RoomID:    roomID,
// 		Room:      room, // add this to fix invalid data error
// 	}

// 	form := forms.New(r.PostForm)

// 	form.Required("first_name", "last_name", "email")
// 	form.MinLength("first_name", 3)
// 	form.IsEmail("email")

// 	if !form.Valid() {
// 		data := make(map[string]interface{})
// 		data["reservation"] = reservation

// 		// add these lines to fix bad data error
// 		stringMap := make(map[string]string)
// 		stringMap["start_date"] = sd
// 		stringMap["end_date"] = ed

// 		render.Template(w, r, "make-reservation.page.tmpl", &models.TemplateData{
// 			Form:      form,
// 			Data:      data,
// 			StringMap: stringMap, // fixes error after invalid data
// 		})
// 		return
// 	}

// 	newReservationID, err := m.DB.InsertReservation(reservation)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't insert reservation into database!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	restriction := models.RoomRestriction{
// 		StartDate:     startDate,
// 		EndDate:       endDate,
// 		RoomID:        roomID,
// 		ReservationID: newReservationID,
// 		RestrictionID: 1,
// 	}

// 	err = m.DB.InsertRoomRestriction(restriction)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't insert room restriction!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	// send notifications - first to guest
// 	htmlMessage := fmt.Sprintf(`
// 		<strong>Reservation Confirmation</strong><br>
// 		Dear %s: <br>
// 		This is confirm your reservation from %s to %s.`,
// 		reservation.FirstName,
// 		reservation.StartDate.Format("2006-01-02"),
// 		reservation.EndDate.Format("2006-01-02"),
// 	)

// 	msg := mailer.Message{
// 		To:      reservation.Email,
// 		From:    "me@here.com",
// 		Subject: "Reservation Confirmation",
// 		Data:    htmlMessage,
// 		//Template: "basic.html",
// 	}

// 	//m.App.MailChan <- msg
// 	m.App.Wait.Add(1)
// 	m.App.Mailer.MailerChan <- msg

// 	// send notification to property owner
// 	htmlMessage = fmt.Sprintf(`
// 		<strong>Reservation Notification</strong>
// 		A reservation has been made for %s from %s to %s.
// `, reservation.Room.RoomName, reservation.StartDate.Format("2006-01-02"), reservation.EndDate.Format("2006-01-02"))

// 	msg = mailer.Message{
// 		To:      "me@here.com",
// 		From:    "me@here.com",
// 		Subject: "Reservation Notification",
// 		Data:    htmlMessage,
// 	}

// 	//m.App.MailChan <- msg
// 	m.App.Wait.Add(1)
// 	m.App.Mailer.MailerChan <- msg

// 	m.App.Session.Put(r.Context(), "reservation", reservation)

// 	http.Redirect(w, r, "/reservation-summary", http.StatusSeeOther)

// }

// // ReservationSummary displays the reservation summary page
// func (m *Repository) ReservationSummary(w http.ResponseWriter, r *http.Request) {
// 	reservation, ok := m.App.Session.Get(r.Context(), "reservation").(models.Reservation)
// 	if !ok {
// 		m.App.Session.Put(r.Context(), "error", "Can't get reservation from session")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	m.App.Session.Remove(r.Context(), "reservation")

// 	data := make(map[string]interface{})
// 	data["reservation"] = reservation

// 	sd := reservation.StartDate.Format("2006-01-02")
// 	ed := reservation.EndDate.Format("2006-01-02")
// 	stringMap := make(map[string]string)
// 	stringMap["start_date"] = sd
// 	stringMap["end_date"] = ed

// 	render.Template(w, r, "reservation-summary.page.tmpl", &models.TemplateData{
// 		Data:      data,
// 		StringMap: stringMap,
// 	})
// }

// type availabilityJSON struct {
// 	OK        bool   `json:"ok"`
// 	Message   string `json:"message"`
// 	RoomID    string `json:"room_id"`
// 	StartDate string `json:"start_date"`
// 	EndDate   string `json:"end_date"`
// }

// // Availability renders the search availability page
// func (m *Repository) Availability(w http.ResponseWriter, r *http.Request) {
// 	render.Template(w, r, "search-availability.page.tmpl", &models.TemplateData{})
// }

// // PostAvailability renders the search availability page
// func (m *Repository) PostAvailability(w http.ResponseWriter, r *http.Request) {
// 	err := r.ParseForm()
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't parse form!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	start := r.Form.Get("start")
// 	end := r.Form.Get("end")

// 	layout := "2006-01-02"
// 	startDate, err := time.Parse(layout, start)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't parse start date!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}
// 	endDate, err := time.Parse(layout, end)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't parse end date!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	rooms, err := m.DB.SearchAvailabilityForAllRooms(startDate, endDate)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "can't get availability for rooms")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	if len(rooms) == 0 {
// 		// no availability
// 		m.App.Session.Put(r.Context(), "error", "No availability")
// 		http.Redirect(w, r, "/search-availability", http.StatusSeeOther)
// 		return
// 	}

// 	data := make(map[string]interface{})
// 	data["rooms"] = rooms

// 	res := models.Reservation{
// 		StartDate: startDate,
// 		EndDate:   endDate,
// 	}

// 	m.App.Session.Put(r.Context(), "reservation", res)

// 	render.Template(w, r, "choose-room.page.tmpl", &models.TemplateData{
// 		Data: data,
// 	})
// }

// // AvailabilityJSON handles request for availability and send JSON response
// func (m *Repository) AvailabilityJSON(w http.ResponseWriter, r *http.Request) {
// 	// need to parse request body
// 	err := r.ParseForm()
// 	if err != nil {
// 		// can't parse form, so return appropriate json
// 		helpers.RespondWithError(w, "Internal server error")
// 		return
// 	}

// 	sd := r.Form.Get("start")
// 	ed := r.Form.Get("end")

// 	fmt.Println(sd)
// 	fmt.Println(ed)

// 	layout := "2006-01-02"
// 	startDate, err := time.Parse(layout, sd)
// 	if err != nil {
// 		helpers.RespondWithError(w, "Error: check start date")
// 		return
// 	}
// 	endDate, err := time.Parse(layout, ed)
// 	if err != nil {
// 		helpers.RespondWithError(w, "Error: check end date")
// 		return
// 	}

// 	roomID, err := strconv.Atoi(r.Form.Get("room_id"))
// 	if err != nil {
// 		helpers.RespondWithError(w, "Error: bad room number")
// 		return
// 	}

// 	available, err := m.DB.SearchAvailabilityByDatesByRoomID(startDate, endDate, roomID)
// 	if err != nil {
// 		// got a database error, so return appropriate json
// 		helpers.RespondWithError(w, "Error: could not find room")
// 		return
// 	}
// 	resp := availabilityJSON{
// 		OK:        available,
// 		Message:   "",
// 		StartDate: sd,
// 		EndDate:   ed,
// 		RoomID:    strconv.Itoa(roomID),
// 	}

// 	// Removed the error check, since we handle all aspects of the json right here
// 	out, _ := json.MarshalIndent(resp, "", "     ")

// 	w.Header().Set("Content-Type", "application/json")
// 	_, err = w.Write(out)
// }

// // ChooseRoom displays list of available rooms
// func (m *Repository) ChooseRoom(w http.ResponseWriter, r *http.Request) {
// 	// split the URL up by /, and grab the 3rd element
// 	exploded := strings.Split(r.RequestURI, "/")
// 	roomID, err := strconv.Atoi(exploded[2])
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "missing url parameter")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	res, ok := m.App.Session.Get(r.Context(), "reservation").(models.Reservation)
// 	if !ok {
// 		m.App.Session.Put(r.Context(), "error", "Can't get reservation from session")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	res.RoomID = roomID

// 	m.App.Session.Put(r.Context(), "reservation", res)

// 	http.Redirect(w, r, "/make-reservation", http.StatusSeeOther)

// }

// // BookRoom takes URL parameters, builds a sessional variable, and takes user to make res screen
// func (m *Repository) BookRoom(w http.ResponseWriter, r *http.Request) {
// 	roomID, _ := strconv.Atoi(r.URL.Query().Get("id"))
// 	sd := r.URL.Query().Get("s")
// 	ed := r.URL.Query().Get("e")

// 	layout := "2006-01-02"
// 	startDate, _ := time.Parse(layout, sd)
// 	endDate, _ := time.Parse(layout, ed)

// 	var res models.Reservation

// 	room, err := m.DB.GetRoomByID(roomID)
// 	if err != nil {
// 		m.App.Session.Put(r.Context(), "error", "Can't get room from db!")
// 		http.Redirect(w, r, "/", http.StatusSeeOther)
// 		return
// 	}

// 	res.Room.RoomName = room.RoomName
// 	res.RoomID = roomID
// 	res.StartDate = startDate
// 	res.EndDate = endDate

// 	m.App.Session.Put(r.Context(), "reservation", res)

// 	http.Redirect(w, r, "/make-reservation", http.StatusSeeOther)
// }

// func (m *Repository) AdminDashboard(w http.ResponseWriter, r *http.Request) {
// 	render.Template(w, r, "admin-dashboard.page.tmpl", &models.TemplateData{})
// }

// // AdminAllReservations shows all reservations inu admin tool
// func (m *Repository) AdminAllReservations(w http.ResponseWriter, r *http.Request) {
// 	reservations, err := m.DB.AllReservations()
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	data := make(map[string]interface{})
// 	data["reservations"] = reservations

// 	render.Template(w, r, "admin-all-reservations.page.tmpl", &models.TemplateData{
// 		Data: data,
// 	})
// }

// // AdminNewReservations shows all new reservations in admin tool
// func (m *Repository) AdminNewReservations(w http.ResponseWriter, r *http.Request) {
// 	reservations, err := m.DB.AllNewReservations()
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	data := make(map[string]interface{})
// 	data["reservations"] = reservations
// 	render.Template(w, r, "admin-new-reservations.page.tmpl", &models.TemplateData{
// 		Data: data,
// 	})
// }

// // AdminShowReservation shows the reservation in the admin tool
// func (m *Repository) AdminShowReservation(w http.ResponseWriter, r *http.Request) {
// 	exploded := strings.Split(r.RequestURI, "/")

// 	id, err := strconv.Atoi(exploded[4])
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	src := exploded[3]

// 	stringMap := make(map[string]string)
// 	stringMap["src"] = src

// 	year := r.URL.Query().Get("y")
// 	month := r.URL.Query().Get("m")

// 	stringMap["month"] = month
// 	stringMap["year"] = year

// 	// get reservation from the database
// 	res, err := m.DB.GetReservationByID(id)
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	data := make(map[string]interface{})
// 	data["reservation"] = res

// 	render.Template(w, r, "admin-reservations-show.page.tmpl", &models.TemplateData{
// 		StringMap: stringMap,
// 		Data:      data,
// 		Form:      forms.New(nil),
// 	})
// }

// // AdminPostShowReservation posts a reservation
// func (m *Repository) AdminPostShowReservation(w http.ResponseWriter, r *http.Request) {
// 	err := r.ParseForm()
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	exploded := strings.Split(r.RequestURI, "/")

// 	id, err := strconv.Atoi(exploded[4])
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	src := exploded[3]

// 	stringMap := make(map[string]string)
// 	stringMap["src"] = src

// 	res, err := m.DB.GetReservationByID(id)
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	res.FirstName = r.Form.Get("first_name")
// 	res.LastName = r.Form.Get("last_name")
// 	res.Email = r.Form.Get("email")
// 	res.Phone = r.Form.Get("phone")

// 	err = m.DB.UpdateReservation(res)
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	month := r.Form.Get("month")
// 	year := r.Form.Get("year")

// 	m.App.Session.Put(r.Context(), "flash", "Changes saved")

// 	if year == "" {
// 		http.Redirect(w, r, fmt.Sprintf("/admin/reservations-%s", src), http.StatusSeeOther)
// 	} else {
// 		http.Redirect(w, r, fmt.Sprintf("/admin/reservations-calendar?y=%s&m=%s", year, month), http.StatusSeeOther)
// 	}
// }

// // AdminReservationsCalendar displays the reservation calendar
// func (m *Repository) AdminReservationsCalendar(w http.ResponseWriter, r *http.Request) {
// 	// assume that there is no month/year specified
// 	now := time.Now()

// 	if r.URL.Query().Get("y") != "" {
// 		year, _ := strconv.Atoi(r.URL.Query().Get("y"))
// 		month, _ := strconv.Atoi(r.URL.Query().Get("m"))
// 		now = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
// 	}

// 	data := make(map[string]interface{})
// 	data["now"] = now

// 	next := now.AddDate(0, 1, 0)
// 	last := now.AddDate(0, -1, 0)

// 	nextMonth := next.Format("01")
// 	nextMonthYear := next.Format("2006")

// 	lastMonth := last.Format("01")
// 	lastMonthYear := last.Format("2006")

// 	stringMap := make(map[string]string)
// 	stringMap["next_month"] = nextMonth
// 	stringMap["next_month_year"] = nextMonthYear
// 	stringMap["last_month"] = lastMonth
// 	stringMap["last_month_year"] = lastMonthYear

// 	stringMap["this_month"] = now.Format("01")
// 	stringMap["this_month_year"] = now.Format("2006")

// 	// get the first and last days of the month
// 	currentYear, currentMonth, _ := now.Date()
// 	currentLocation := now.Location()
// 	firstOfMonth := time.Date(currentYear, currentMonth, 1, 0, 0, 0, 0, currentLocation)
// 	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)

// 	intMap := make(map[string]int)
// 	intMap["days_in_month"] = lastOfMonth.Day()

// 	rooms, err := m.DB.AllRooms()
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	data["rooms"] = rooms

// 	for _, x := range rooms {
// 		// create maps
// 		reservationMap := make(map[string]int)
// 		blockMap := make(map[string]int)

// 		for d := firstOfMonth; d.After(lastOfMonth) == false; d = d.AddDate(0, 0, 1) {
// 			reservationMap[d.Format("2006-01-2")] = 0
// 			blockMap[d.Format("2006-01-2")] = 0
// 		}

// 		// get all the restrictions for the current room
// 		restrictions, err := m.DB.GetRestrictionsForRoomByDate(x.ID, firstOfMonth, lastOfMonth)
// 		if err != nil {
// 			helpers.ServerError(w, err)
// 			return
// 		}

// 		for _, y := range restrictions {
// 			if y.ReservationID > 0 {
// 				// it's a reservation
// 				for d := y.StartDate; d.After(y.EndDate) == false; d = d.AddDate(0, 0, 1) {
// 					reservationMap[d.Format("2006-01-2")] = y.ReservationID
// 				}
// 			} else {
// 				// it's a block
// 				blockMap[y.StartDate.Format("2006-01-2")] = y.ID
// 			}
// 		}
// 		data[fmt.Sprintf("reservation_map_%d", x.ID)] = reservationMap
// 		data[fmt.Sprintf("block_map_%d", x.ID)] = blockMap

// 		m.App.Session.Put(r.Context(), fmt.Sprintf("block_map_%d", x.ID), blockMap)
// 	}

// 	render.Template(w, r, "admin-reservations-calendar.page.tmpl", &models.TemplateData{
// 		StringMap: stringMap,
// 		Data:      data,
// 		IntMap:    intMap,
// 	})
// }

// // AdminProcessReservation  marks a reservation as processed
// func (m *Repository) AdminProcessReservation(w http.ResponseWriter, r *http.Request) {
// 	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
// 	src := chi.URLParam(r, "src")
// 	err := m.DB.UpdateProcessedForReservation(id, 1)
// 	if err != nil {
// 		log.Println(err)
// 	}

// 	year := r.URL.Query().Get("y")
// 	month := r.URL.Query().Get("m")

// 	m.App.Session.Put(r.Context(), "flash", "Reservation marked as processed")

// 	if year == "" {
// 		http.Redirect(w, r, fmt.Sprintf("/admin/reservations-%s", src), http.StatusSeeOther)
// 	} else {
// 		http.Redirect(w, r, fmt.Sprintf("/admin/reservations-calendar?y=%s&m=%s", year, month), http.StatusSeeOther)
// 	}
// }

// // AdminDeleteReservation deletes a reservation
// func (m *Repository) AdminDeleteReservation(w http.ResponseWriter, r *http.Request) {
// 	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
// 	src := chi.URLParam(r, "src")
// 	_ = m.DB.DeleteReservation(id)

// 	year := r.URL.Query().Get("y")
// 	month := r.URL.Query().Get("m")

// 	m.App.Session.Put(r.Context(), "flash", "Reservation deleted")

// 	if year == "" {
// 		http.Redirect(w, r, fmt.Sprintf("/admin/reservations-%s", src), http.StatusSeeOther)
// 	} else {
// 		http.Redirect(w, r, fmt.Sprintf("/admin/reservations-calendar?y=%s&m=%s", year, month), http.StatusSeeOther)
// 	}
// }

// // AdminPostReservationsCalendar handles post of reservation calendar
// func (m *Repository) AdminPostReservationsCalendar(w http.ResponseWriter, r *http.Request) {
// 	err := r.ParseForm()
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	year, _ := strconv.Atoi(r.Form.Get("y"))
// 	month, _ := strconv.Atoi(r.Form.Get("m"))

// 	// process blocks
// 	rooms, err := m.DB.AllRooms()
// 	if err != nil {
// 		helpers.ServerError(w, err)
// 		return
// 	}

// 	form := forms.New(r.PostForm)

// 	for _, x := range rooms {
// 		// Get the block map from the session. Loop through entire map, if we have an entry in the map
// 		// that does not exist in our posted data, and if the restriction id > 0, then it is a block we need to
// 		// remove.
// 		curMap := m.App.Session.Get(r.Context(), fmt.Sprintf("block_map_%d", x.ID)).(map[string]int)
// 		for name, value := range curMap {
// 			// ok will be false if the value is not in the map
// 			if val, ok := curMap[name]; ok {
// 				// only pay attention to values > 0, and that are not in the form post
// 				// the rest are just placeholders for days without blocks
// 				if val > 0 {
// 					if !form.Has(fmt.Sprintf("remove_block_%d_%s", x.ID, name)) {
// 						// delete the restriction by id
// 						err := m.DB.DeleteBlockByID(value)
// 						if err != nil {
// 							log.Println(err)
// 						}
// 					}
// 				}
// 			}
// 		}
// 	}

// 	// now handle new blocks
// 	for name := range r.PostForm {
// 		if strings.HasPrefix(name, "add_block") {
// 			exploded := strings.Split(name, "_")
// 			roomID, _ := strconv.Atoi(exploded[2])
// 			t, _ := time.Parse("2006-01-2", exploded[3])
// 			// insert a new block
// 			err := m.DB.InsertBlockForRoom(roomID, t)
// 			if err != nil {
// 				log.Println(err)
// 			}
// 		}
// 	}

// 	m.App.Session.Put(r.Context(), "flash", "Changes saved")
// 	http.Redirect(w, r, fmt.Sprintf("/admin/reservations-calendar?y=%d&m=%d", year, month), http.StatusSeeOther)
// }
