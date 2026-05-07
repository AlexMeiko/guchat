package entity

import (
	"database/sql"
	"time"
)

const (
	ToolCallStatusPending = "pending"
	ToolCallStatusRunning = "running"
	ToolCallStatusDone    = "done"
	ToolCallStatusFailed  = "failed"
)

type ToolCall struct {
	ID                 int64        `db:"id"`
	ConversationID     string       `db:"conversation_id"`
	AssistantMessageID string       `db:"assistant_message_id"`
	ProviderCallID     string       `db:"provider_call_id"`
	ToolName           string       `db:"tool_name"`
	ArgumentsJSON      string       `db:"arguments_json"`
	ResultJSON         string       `db:"result_json"`
	Status             string       `db:"status"`
	ErrorMessage       string       `db:"error_message"`
	Round              int          `db:"round"`
	Seq                int          `db:"seq"`
	StartedAt          sql.NullTime `db:"started_at"`
	FinishedAt         sql.NullTime `db:"finished_at"`
	CreatedAt          time.Time    `db:"created_at"`
}
