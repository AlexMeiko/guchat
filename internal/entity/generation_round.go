package entity

import "time"

type GenerationRound struct {
	ID                   int64     `db:"id"`
	ConversationID       string    `db:"conversation_id"`
	AssistantMessageID   string    `db:"assistant_message_id"`
	Round                int       `db:"round"`
	ContentStartOffset   int       `db:"content_start_offset"`
	ContentEndOffset     int       `db:"content_end_offset"`
	ReasoningStartOffset int       `db:"reasoning_start_offset"`
	ReasoningEndOffset   int       `db:"reasoning_end_offset"`
	CreatedAt            time.Time `db:"created_at"`
}
