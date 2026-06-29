package entity

import "time"

const (
	MessageRoleSystem    = "system"
	MessageRoleUser      = "user"
	MessageRoleAssistant = "assistant"
)

const (
	MessageStatusPending   = "pending"
	MessageStatusStreaming = "streaming"
	MessageStatusDone      = "done"
	MessageStatusFailed    = "failed"
)

type Message struct {
	ID               string    `db:"id"`
	ConversationID   string    `db:"conversation_id"`
	Role             string    `db:"role"`
	Content          string    `db:"content"`
	ReasoningContent string    `db:"reasoning_content"`
	SummaryContent   string    `db:"summary_content"`
	HasSummary       bool      `db:"has_summary"`
	Status           string    `db:"status"`
	ErrorMessage     string    `db:"error_message"`
	Seq              int       `db:"seq"`
	CreatedAt        time.Time `db:"created_at"`
}
