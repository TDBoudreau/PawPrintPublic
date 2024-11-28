package diplomapdfs

import (
	"errors"
	"sync"
	"time"
)

// ProgressUpdate represents a progress update for a task
type ProgressUpdate struct {
	Status   string `json:"status"`
	Progress int    `json:"progress"` // Percentage completion
	Error    string `json:"error,omitempty"`
}

// Task represents a long-running task
type Task struct {
	ID           string
	ProgressChan chan ProgressUpdate
	DoneChan     chan struct{}
	StartedAt    time.Time
	FinishedAt   time.Time
}

// TaskManager manages tasks and their progress
type TaskManager struct {
	Tasks map[string]*Task
	Mu    *sync.RWMutex
}

// NewTaskManager creates a new TaskManager
func NewTaskManager() *TaskManager {
	return &TaskManager{
		Tasks: make(map[string]*Task),
		Mu:    &sync.RWMutex{},
	}
}

// CreateTask initializes a new task
func (tm *TaskManager) CreateTask(taskID string) *Task {
	tm.Mu.Lock()
	defer tm.Mu.Unlock()
	task := &Task{
		ID:           taskID,
		ProgressChan: make(chan ProgressUpdate),
		DoneChan:     make(chan struct{}),
		StartedAt:    time.Now(),
	}
	tm.Tasks[taskID] = task
	return task
}

// GetTask retrieves a task by ID
func (tm *TaskManager) GetTask(taskID string) (*Task, error) {
	tm.Mu.RLock()
	defer tm.Mu.RUnlock()
	task, exists := tm.Tasks[taskID]
	if !exists {
		return nil, errors.New("task not found")
	}
	return task, nil
}

// DeleteTask removes a task from the manager
func (tm *TaskManager) DeleteTask(taskID string) {
	tm.Mu.Lock()
	defer tm.Mu.Unlock()
	delete(tm.Tasks, taskID)
}
