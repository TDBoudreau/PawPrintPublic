package helpers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"pawprintpublic/internal/config"
	"runtime/debug"
)

var app *config.AppConfig

// NewHelpers sets up app config for helpers
func NewHelpers(a *config.AppConfig) {
	app = a
}

func ClientError(w http.ResponseWriter, status int) {
	app.InfoLog.Println("Client error with status of", status)
	http.Error(w, http.StatusText(status), status)
}

func ServerError(w http.ResponseWriter, err error) {
	trace := fmt.Sprintf("%s\n%s", err.Error(), debug.Stack())
	app.ErrorLog.Println(trace)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func IsAuthenticated(r *http.Request) bool {
	exists := app.Session.Exists(r.Context(), "user_id")
	return exists
}

func IsValidEmail(email string) bool {
	// Implement email validation
	return true
}

type jsonResponse struct {
	OK        bool   `json:"ok"`
	Message   string `json:"message"`
	RoomID    string `json:"room_id"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
}

func RespondWithError(w http.ResponseWriter, message string) {
	resp := jsonResponse{
		OK:      false,
		Message: message,
	}
	out, _ := json.MarshalIndent(resp, "", "     ")
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

// Helper function to validate Excel file extensions
func IsValidExcelFile(filename string) bool {
	ext := filepath.Ext(filename)
	switch ext {
	case ".xlsx", ".xls":
		return true
	default:
		return false
	}
}
