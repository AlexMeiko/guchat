package stream

import (
	"context"
	"strings"
	"sync"

	"github.com/AlexMeiko/guchat/internal/entity"
)

type Snapshot struct {
	MessageID        string
	Status           string
	Content          string
	ReasoningContent string
	ErrorMessage     string
}

type Task struct {
	mu               sync.RWMutex
	messageID        string
	status           string
	content          strings.Builder
	reasoningContent strings.Builder
	errorMessage     string
	cancel           context.CancelFunc
}

func NewTask(messageID string, cancel context.CancelFunc) *Task {
	return &Task{
		messageID: messageID,
		status:    entity.MessageStatusPending,
		cancel:    cancel,
	}
}

func (t *Task) Start() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.status = entity.MessageStatusStreaming
}

func (t *Task) Cancel(errMsg string) {
	t.mu.Lock()

	if t.status == entity.MessageStatusDone || t.status == entity.MessageStatusFailed {
		t.mu.Unlock()
		return
	}

	t.status = entity.MessageStatusFailed
	t.errorMessage = errMsg
	cancel := t.cancel
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (t *Task) Done() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// cancelled
	if t.status == entity.MessageStatusFailed {
		return
	}

	t.status = entity.MessageStatusDone
	t.errorMessage = ""
}

func (t *Task) Failed(errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// cancelled
	if t.status == entity.MessageStatusFailed && t.errorMessage != "" {
		return
	}

	t.status = entity.MessageStatusFailed
	t.errorMessage = errMsg
}

func (t *Task) AppendContent(delta string) {
	if delta == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.content.WriteString(delta)
}

func (t *Task) AppendReasoningContent(delta string) {
	if delta == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.reasoningContent.WriteString(delta)
}

func (t *Task) Snapshot() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return Snapshot{
		MessageID:        t.messageID,
		Status:           t.status,
		Content:          t.content.String(),
		ReasoningContent: t.reasoningContent.String(),
		ErrorMessage:     t.errorMessage,
	}
}

type Manager struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*Task),
	}
}

func (m *Manager) Create(messageID string, cancel context.CancelFunc) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.tasks[messageID] = NewTask(messageID, cancel)
	return m.tasks[messageID]
}

func (m *Manager) Get(messageID string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[messageID]
	return task, ok
}

func (m *Manager) Delete(messageID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tasks, messageID)
}
