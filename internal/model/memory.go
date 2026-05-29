package model

import (
	"encoding/json"
	"time"
)

type UpdateMemoryStatusRequest struct {
	Status string `json:"status" binding:"required"`
}

type MemoryItemResponse struct {
	ID             int64           `json:"id"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Scope          string          `json:"scope"`
	Category       string          `json:"category"`
	Origin         string          `json:"origin"`
	SourceType     string          `json:"source_type"`
	SourceRef      string          `json:"source_ref,omitempty"`
	SourceTitle    string          `json:"source_title,omitempty"`
	Content        string          `json:"content"`
	Metadata       json.RawMessage `json:"metadata"`
	Confidence     float64         `json:"confidence"`
	ExpiresAt      *time.Time      `json:"expires_at,omitempty"`
	Status         string          `json:"status"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ListMemoryItemsResponse struct {
	Items []MemoryItemResponse `json:"items"`
}
