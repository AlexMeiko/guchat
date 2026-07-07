package sandbox

import (
	"container/heap"
	"context"
	"errors"
	"sync"
	"time"
)

var ErrTerminalNotOpen = errors.New("terminal not open")

const (
	terminalCleanupBudget = 5 * time.Second
)

type terminalState struct {
	expiresAt time.Time
	running   int
}

type Manager struct {
	workspaceManager *WorkspaceManager
	runner           Runner
	idleTimeout      time.Duration

	mu              sync.Mutex
	activeTerminals map[terminalKey]terminalState
	expireHeap      terminalExpireHeap
}

type terminalKey struct {
	userID         int64
	conversationID string
}

type Runner interface {
	Open(ctx context.Context, input OpenInput) error
	Exec(ctx context.Context, userID int64, conversationID string, input ExecInput) (*ExecResult, error)
	Destroy(ctx context.Context, userID int64, conversationID string) error
}

type OpenInput struct {
	UserID         int64
	ConversationID string
	WorkspacePath  string
}

type ExecInput struct {
	Command string
	Timeout time.Duration
}

type ExecResult struct {
	ExitCode  int
	Stdout    string
	Stderr    string
	Duration  time.Duration
	TimedOut  bool
	Truncated bool
}

func NewManager(workspaceManager *WorkspaceManager, runner Runner, idleTimeout time.Duration) *Manager {
	if idleTimeout <= 0 {
		idleTimeout = 30 * time.Minute
	}

	return &Manager{
		workspaceManager: workspaceManager,
		runner:           runner,
		idleTimeout:      idleTimeout,
		activeTerminals:  make(map[terminalKey]terminalState),
	}
}

func (m *Manager) Open(ctx context.Context, userID int64, conversationID string) error {
	workspacePath, err := m.workspaceManager.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return err
	}

	if err := m.runner.Open(ctx, OpenInput{
		UserID:         userID,
		ConversationID: conversationID,
		WorkspacePath:  workspacePath,
	}); err != nil {
		return err
	}

	m.touchTerminal(userID, conversationID, m.idleTimeout)
	return nil
}

func (m *Manager) Exec(
	ctx context.Context,
	userID int64,
	conversationID string,
	input ExecInput,
) (*ExecResult, error) {
	if input.Timeout <= 0 {
		input.Timeout = 30 * time.Second
	}

	m.markTerminalRunning(userID, conversationID)
	defer m.markTerminalDone(userID, conversationID, m.idleTimeout)

	return m.runner.Exec(ctx, userID, conversationID, input)
}

func (m *Manager) Destroy(ctx context.Context, userID int64, conversationID string) error {
	destroyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := m.runner.Destroy(destroyCtx, userID, conversationID)
	cancel()
	if err != nil {
		return err
	}

	key := terminalKey{userID: userID, conversationID: conversationID}

	m.mu.Lock()
	delete(m.activeTerminals, key)
	m.mu.Unlock()

	return nil
}

// 清理逻辑 lazy heap + map

func (m *Manager) touchTerminal(userID int64, conversationID string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := terminalKey{userID: userID, conversationID: conversationID}
	expiresAt := time.Now().Add(ttl)

	state, exists := m.activeTerminals[key]
	state.expiresAt = expiresAt
	m.activeTerminals[key] = state

	if !exists {
		heap.Push(&m.expireHeap, terminalExpireItem{
			key:       key,
			expiresAt: expiresAt,
		})
	}
}

func (m *Manager) markTerminalRunning(userID int64, conversationID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := terminalKey{userID: userID, conversationID: conversationID}

	state, exists := m.activeTerminals[key]
	if !exists {
		return
	}

	state.running++
	m.activeTerminals[key] = state
}

func (m *Manager) markTerminalDone(userID int64, conversationID string, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := terminalKey{userID: userID, conversationID: conversationID}

	state, exists := m.activeTerminals[key]
	if !exists {
		return
	}

	if state.running > 0 {
		state.running--
	}
	state.expiresAt = time.Now().Add(ttl)
	m.activeTerminals[key] = state
}

func (m *Manager) CleanupExpired(ctx context.Context) {
	deadline := time.Now().Add(terminalCleanupBudget)

	m.mu.Lock()
	defer m.mu.Unlock()

	for {
		now := time.Now()
		item, ok := m.expireHeap.Peek()
		if !ok || now.Before(item.expiresAt) {
			return
		}

		heap.Pop(&m.expireHeap)

		state, exists := m.activeTerminals[item.key]
		if !exists {
			continue
		}

		if !state.expiresAt.Equal(item.expiresAt) {
			heap.Push(&m.expireHeap, terminalExpireItem{
				key:       item.key,
				expiresAt: state.expiresAt,
			})
			continue
		}

		if state.running > 0 {
			state.expiresAt = now.Add(m.idleTimeout)
			m.activeTerminals[item.key] = state

			heap.Push(&m.expireHeap, terminalExpireItem{
				key:       item.key,
				expiresAt: state.expiresAt,
			})
			continue
		}

		if err := m.runner.Destroy(ctx, item.key.userID, item.key.conversationID); err != nil {
			heap.Push(&m.expireHeap, terminalExpireItem{
				key:       item.key,
				expiresAt: state.expiresAt,
			})
			return
		}

		delete(m.activeTerminals, item.key)

		if time.Now().After(deadline) {
			return
		}
	}
}
