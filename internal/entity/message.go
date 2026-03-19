package entity

import "time"

type Message struct {
	ID               string    `db:"id"`
	ConversationID   string    `db:"conversation_id"`
	Role             string    `db:"role"`
	Content          string    `db:"content"`
	ReasoningContent string    `db:"reasoning_content"`
	Status           string    `db:"status"`
	ErrorMessage     string    `db:"error_message"`
	Seq              int       `db:"seq"`
	CreatedAt        time.Time `db:"created_at"`
}
