package model

import "time"

type CreateConversationRequest struct {
	Title string `json:"title"`
}

type UpdateConversationRequest struct {
	Title string `json:"title" binding:"required"`
}

type ConversationResponse struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
