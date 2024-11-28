package dbrepo

import (
	"context"
	"time"
)

// InsertFile stores a file in the database
func (m *sqliteDBRepo) InsertFile(taskID, sessionID, fileName, fileType string, fileData []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `INSERT INTO files (task_id, session_id, file_name, file_type, file_data) VALUES (?, ?, ?, ?, ?)`
	_, err := m.DB.ExecContext(ctx, query, taskID, sessionID, fileName, fileType, fileData)
	return err
}

// GetFile retrieves a file from the database based on taskID and fileType
func (m *sqliteDBRepo) GetFile(taskID, fileType string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `SELECT file_data FROM files WHERE task_id = ? AND file_type = ?`
	var fileData []byte
	err := m.DB.QueryRowContext(ctx, query, taskID, fileType).Scan(&fileData)
	return fileData, err
}

// DeleteFilesByTask deletes files associated with a task
func (m *sqliteDBRepo) DeleteFilesByTask(taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `DELETE FROM files WHERE task_id = ?`
	_, err := m.DB.ExecContext(ctx, query, taskID)
	return err
}

// DeleteOldFiles deletes files older than the specified duration
func (m *sqliteDBRepo) DeleteOldFiles(olderThan time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Calculate the cutoff time in UTC
	cutoff := time.Now().UTC().Add(-olderThan).Format("2006-01-02 15:04:05")
	query := `DELETE FROM files WHERE upload_time <= ?`

	// Execute the DELETE statement
	_, err := m.DB.ExecContext(ctx, query, cutoff)
	return err
}
