package model

type CreateGenerationRequest struct {
	ModelID int64 `json:"model_id" binding:"required"`
}

type GenerationEvent struct {
	MessageID        string `json:"message_id"`
	Status           string `json:"status,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Delta            string `json:"delta,omitempty"`
	Error            string `json:"error,omitempty"`
	ContentBytes     int    `json:"content_bytes,omitempty"`
	ReasoningBytes   int    `json:"reasoning_bytes,omitempty"`
}
