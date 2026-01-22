package shared

import (
	"sync"
	"time"
)

// TaskStatus represents the current state of a task
type TaskStatus struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Progress    int        `json:"progress"` // 0-100
	Completed   bool       `json:"completed"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// TaskManager manages all tasks with thread-safe operations
type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*TaskStatus
}

func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]*TaskStatus),
	}
}

func (tm *TaskManager) CreateTask(id, title string) *TaskStatus {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	task := &TaskStatus{
		ID:        id,
		Title:     title,
		Progress:  0,
		Completed: false,
		StartedAt: time.Now(),
	}
	tm.tasks[id] = task
	return task
}

func (tm *TaskManager) GetTask(id string) (*TaskStatus, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	task, exists := tm.tasks[id]
	return task, exists
}

func (tm *TaskManager) UpdateProgress(id string, progress int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if task, exists := tm.tasks[id]; exists {
		task.Progress = progress
		if progress >= 100 {
			task.Completed = true
			now := time.Now()
			task.CompletedAt = &now
		}
	}
}
