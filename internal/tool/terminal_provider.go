package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/sandbox"
	"github.com/AlexMeiko/guchat/internal/service"
)

const (
	ToolTerminalOpen = "terminal_open"
	ToolTerminalExec = "terminal_exec"

	sandboxDescriptionPath = "/etc/guchat-sandbox-capabilities.txt"
)

type terminalOpenResult struct {
	OK          bool   `json:"ok"`
	Description string `json:"description,omitempty"`
}

type terminalExecArgs struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type terminalExecResult struct {
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"duration_ms"`
	TimedOut   bool   `json:"timed_out"`
	Truncated  bool   `json:"truncated"`
}

type terminalErrorResult struct {
	OK        bool   `json:"ok"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

type TerminalProvider struct {
	manager *sandbox.Manager
}

func NewTerminalProvider(manager *sandbox.Manager) *TerminalProvider {
	return &TerminalProvider{manager: manager}
}

func (p *TerminalProvider) Name() string {
	return "terminal"
}

func (p *TerminalProvider) ListTools(ctx context.Context, user service.UserContext) ([]service.ToolDefinition, error) {
	return []service.ToolDefinition{
		{
			Name:        ToolTerminalOpen,
			Description: "打开或重新打开当前会话的临时终端容器。容器可能因空闲超时被回收；/workspace 会保留用户上传文件和需要保留的生成文件。如果容器因超时被销毁，容器内其他位置以及临时安装的软件包不会保留。如果打开结果包含 description 字段，应优先遵循其中的环境和文件编辑要求。",
			Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {},
                "additionalProperties": false
            }`),
		},
		{
			Name:        ToolTerminalExec,
			Description: "在已打开的当前会话终端容器中执行一条 shell 命令。如果返回 terminal_not_open，先重新打开终端容器再重试命令。每次调用都是新的 shell；如果需要进入子目录，请在命令中使用 cd，例如 cd project && npm test。容器是临时环境，如果因超时被销毁，临时安装的软件包不会保留。需要在后续消息继续使用、需要用户下载、或需要保留的文件必须写入 /workspace；容器内其他位置可能随容器销毁而丢失。",
			Parameters: json.RawMessage(`{
                "type": "object",
                "properties": {
                    "command": {
                        "type": "string",
                        "description": "要执行的 shell 命令"
                    },
                    "timeout_seconds": {
                        "type": "integer",
                        "description": "命令超时时间，单位秒。未传或小于等于 0 时默认 30 秒"
                    }
                },
                "required": ["command"],
                "additionalProperties": false
			}`),
		},
	}, nil
}

func (p *TerminalProvider) CallTool(ctx context.Context, user service.UserContext, name string, args json.RawMessage) (service.ToolResult, error) {
	switch name {
	case ToolTerminalOpen:
		return p.openTerminal(ctx, user)
	case ToolTerminalExec:
		return p.execTerminal(ctx, user, args)
	default:
		return service.ToolResult{}, service.ErrToolNotFound
	}
}

func (p *TerminalProvider) openTerminal(ctx context.Context, user service.UserContext) (service.ToolResult, error) {
	if user.UserID <= 0 || strings.TrimSpace(user.ConversationID) == "" {
		return service.ToolResult{}, errors.New("terminal requires user and conversation context")
	}

	if err := p.manager.Open(ctx, user.UserID, user.ConversationID); err != nil {
		return service.ToolResult{}, err
	}

	result := terminalOpenResult{
		OK: true,
	}

	description, err := p.manager.Exec(ctx, user.UserID, user.ConversationID, sandbox.ExecInput{
		Command: "if [ -r " + sandboxDescriptionPath + " ]; then cat " + sandboxDescriptionPath + "; fi",
		Timeout: 5 * time.Second,
	})
	if err == nil && description.ExitCode == 0 {
		content := strings.TrimSpace(description.Stdout)
		if content != "" {
			result.Description = content
		}
	}

	return marshalTerminalToolResult(ToolTerminalOpen, result)
}

func (p *TerminalProvider) execTerminal(ctx context.Context, user service.UserContext, args json.RawMessage) (service.ToolResult, error) {
	if user.UserID <= 0 || strings.TrimSpace(user.ConversationID) == "" {
		return service.ToolResult{}, errors.New("terminal requires user and conversation context")
	}

	var input terminalExecArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return service.ToolResult{}, err
	}

	command := strings.TrimSpace(input.Command)
	if command == "" {
		return service.ToolResult{}, errors.New("command is required")
	}

	result, err := p.manager.Exec(ctx, user.UserID, user.ConversationID, sandbox.ExecInput{
		Command: command,
		Timeout: time.Duration(input.TimeoutSeconds) * time.Second,
	})
	if errors.Is(err, sandbox.ErrTerminalNotOpen) {
		return marshalTerminalToolResult(ToolTerminalExec, terminalErrorResult{
			OK:        false,
			ErrorCode: "terminal_not_open",
			Message:   "Terminal is not open or has expired. Call terminal_open before executing commands. Files in /workspace are still available.",
		})
	}
	if err != nil {
		return service.ToolResult{}, err
	}

	return marshalTerminalToolResult(ToolTerminalExec, terminalExecResult{
		OK:         true,
		ExitCode:   result.ExitCode,
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		DurationMS: result.Duration.Milliseconds(),
		TimedOut:   result.TimedOut,
		Truncated:  result.Truncated,
	})
}

func marshalTerminalToolResult(name string, value any) (service.ToolResult, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return service.ToolResult{}, err
	}

	return service.ToolResult{
		Name:   name,
		Result: payload,
	}, nil
}
