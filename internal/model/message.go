package model

import "time"

type CreateMessageRequest struct {
	Content string `json:"content" binding:"required"`
	PrevID  string `json:"prev_id"`
}

type UpdateMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

type MessageResponse struct {
	ID               string          `json:"id"`
	ConversationID   string          `json:"conversation_id"`
	Role             string          `json:"role"`
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content"`
	ToolCalls        []ToolCallEvent `json:"tool_calls,omitempty"`
	Status           string          `json:"status"`
	ErrorMessage     string          `json:"error_message"`
	CreatedAt        time.Time       `json:"created_at"`
}

type ListMessagesResponse struct {
	Items         []MessageResponse `json:"items"`
	HasMore       bool              `json:"has_more"`
	NextBeforeSeq *int              `json:"next_before_seq,omitempty"`
}
