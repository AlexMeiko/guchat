package sandbox

import (
	"context"
	"errors"
	"time"
)

var ErrTerminalNotOpen = errors.New("terminal not open")

type Manager struct {
	workspaceManager *WorkspaceManager
	runner           Runner
}

type Runner interface {
	Open(ctx context.Context, input OpenInput) error
	Exec(ctx context.Context, userID int64, conversationID string, input ExecInput) (*ExecResult, error)
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

func NewManager(workspaceManager *WorkspaceManager, runner Runner) *Manager {
	return &Manager{
		workspaceManager: workspaceManager,
		runner:           runner,
	}
}

func (m *Manager) Open(ctx context.Context, userID int64, conversationID string) error {
	workspacePath, err := m.workspaceManager.EnsureWorkspace(userID, conversationID)
	if err != nil {
		return err
	}

	return m.runner.Open(ctx, OpenInput{
		UserID:         userID,
		ConversationID: conversationID,
		WorkspacePath:  workspacePath,
	})
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

	return m.runner.Exec(ctx, userID, conversationID, input)
}
