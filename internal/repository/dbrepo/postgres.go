package dbrepo

import (
	"context"
	"time"
)

// InsertFile stores a file in the database
func (m *postgresDBRepo) InsertFile(taskID, sessionID, fileName, fileType string, fileData []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `INSERT INTO files (task_id, session_id, file_name, file_type, file_data)
	          VALUES ($1, $2, $3, $4, $5)`
	_, err := m.DB.ExecContext(ctx, query, taskID, sessionID, fileName, fileType, fileData)
	return err
}

// GetFile retrieves a file from the database based on taskID and fileType
func (m *postgresDBRepo) GetFile(taskID, fileType string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `SELECT file_data FROM files WHERE task_id = $1 AND file_type = $2`
	var fileData []byte
	err := m.DB.QueryRowContext(ctx, query, taskID, fileType).Scan(&fileData)
	return fileData, err
}

// DeleteFilesByTask deletes files associated with a task
func (m *postgresDBRepo) DeleteFilesByTask(taskID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	query := `DELETE FROM files WHERE task_id = $1`
	_, err := m.DB.ExecContext(ctx, query, taskID)
	return err
}

// DeleteOldFiles deletes files older than the specified duration
func (m *postgresDBRepo) DeleteOldFiles(olderThan time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Calculate the cutoff time
	cutoff := time.Now().UTC().Add(-olderThan)
	query := `DELETE FROM files WHERE upload_time <= $1`

	// Execute the DELETE statement
	_, err := m.DB.ExecContext(ctx, query, cutoff)
	return err
}
