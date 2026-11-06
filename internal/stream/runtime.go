package stream

import (
	"context"
	"strings"
	"sync"

	"github.com/AlexMeiko/guchat/internal/entity"
)

type ToolCallSnapshot struct {
	ID           int64
	ProviderID   string
	Name         string
	Arguments    string
	Result       string
	Status       string
	ErrorMessage string
	Round        int
	Seq          int
}

type Snapshot struct {
	MessageID        string
	Status           string
	Content          string
	ReasoningContent string
	ToolCalls        []ToolCallSnapshot
	ErrorMessage     string
}

type Task struct {
	mu               sync.RWMutex
	messageID        string
	status           string
	content          strings.Builder
	reasoningContent strings.Builder
	toolCalls        []ToolCallSnapshot
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

func (t *Task) Start() bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.status != entity.MessageStatusPending {
		return false
	}

	t.status = entity.MessageStatusStreaming
	return true
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

func (t *Task) AddToolCall(call ToolCallSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.toolCalls = append(t.toolCalls, call)
}

func (t *Task) UpdateToolCallRunning(id int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.toolCalls {
		if t.toolCalls[i].ID == id {
			t.toolCalls[i].Status = entity.ToolCallStatusRunning
			return
		}
	}
}

func (t *Task) UpdateToolCallDone(id int64, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.toolCalls {
		if t.toolCalls[i].ID == id {
			t.toolCalls[i].Status = entity.ToolCallStatusDone
			t.toolCalls[i].Result = result
			return
		}
	}
}

func (t *Task) UpdateToolCallFailed(id int64, result string, errorMessage string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i := range t.toolCalls {
		if t.toolCalls[i].ID == id {
			t.toolCalls[i].Status = entity.ToolCallStatusFailed
			t.toolCalls[i].Result = result
			t.toolCalls[i].ErrorMessage = errorMessage
			return
		}
	}
}

func (t *Task) Snapshot() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()

	toolCalls := make([]ToolCallSnapshot, len(t.toolCalls))
	copy(toolCalls, t.toolCalls)

	return Snapshot{
		MessageID:        t.messageID,
		Status:           t.status,
		Content:          t.content.String(),
		ReasoningContent: t.reasoningContent.String(),
		ToolCalls:        toolCalls,
		ErrorMessage:     t.errorMessage,
	}
}

type Event struct {
	Type      EventType
	MessageID string
	Message   *entity.Message
	Task      *Task
}

type Manager struct {
	mu          sync.RWMutex
	tasks       map[string]*Task
	subscribers map[string]map[chan Event]struct{}
}

func NewManager() *Manager {
	return &Manager{
		tasks:       make(map[string]*Task),
		subscribers: make(map[string]map[chan Event]struct{}),
	}
}

func (m *Manager) Has(conversationID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.tasks[conversationID]
	return ok
}

func (m *Manager) Create(conversationID, messageID string, cancel context.CancelFunc) *Task {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tasks[conversationID] != nil {
		return nil
	}

	m.tasks[conversationID] = NewTask(messageID, cancel)
	return m.tasks[conversationID]
}

func (m *Manager) Get(conversationID, messageID string) (*Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[conversationID]
	if !ok || task.messageID != messageID {
		return nil, false
	}
	return task, ok
}

func (m *Manager) CancelIfCurrent(conversationID, messageID, msg string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[conversationID]
	if !ok {
		return "", false
	}
	if task.messageID != messageID {
		return task.messageID, false
	}

	task.Cancel(msg)
	return task.messageID, true
}

func (m *Manager) Delete(conversationID, messageID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, ok := m.tasks[conversationID]
	if !ok || task.messageID != messageID {
		return false
	}

	delete(m.tasks, conversationID)
	return true
}

func (m *Manager) Subscribe(conversationID string) (
	current *Task,
	events <-chan Event,
	unsubscribe func(),
) {
	eventChan := make(chan Event, 16)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.subscribers[conversationID] == nil {
		m.subscribers[conversationID] = make(map[chan Event]struct{})
	}

	m.subscribers[conversationID][eventChan] = struct{}{}
	var once sync.Once

	unsubscribe = func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()

			subscribers := m.subscribers[conversationID]
			if subscribers == nil {
				return
			}

			if _, ok := subscribers[eventChan]; !ok {
				// Publish 已经移除并关闭了这个 channel
				return
			}

			// 删对应的订阅 channel
			delete(subscribers, eventChan)

			//如果这个会话都没人订阅了，就直接把第二维map删掉
			if len(subscribers) == 0 {
				delete(m.subscribers, conversationID)
			}

			close(eventChan)
		})
	}

	current = m.tasks[conversationID]
	return current, eventChan, unsubscribe
}

func (m *Manager) Publish(conversationID string, event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	subscribers := m.subscribers[conversationID]

	for eventChan := range subscribers {
		select {
		case eventChan <- event:
		default:
			// 慢连接无法继续接收，关闭它让客户端重连并重新同步
			delete(subscribers, eventChan)
			close(eventChan)
		}
	}

	if len(subscribers) == 0 {
		delete(m.subscribers, conversationID)
	}
}
