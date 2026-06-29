package model

type CreateGenerationRequest struct {
	ModelID  int64  `json:"model_id" binding:"required"`
	ToolMode string `json:"tool_mode"`
}

type ToolCallEvent struct {
	ID           int64  `json:"id"`
	ProviderID   string `json:"provider_id"`
	Name         string `json:"name"`
	Arguments    string `json:"arguments"`
	Result       string `json:"result,omitempty"`
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
	Round        int    `json:"round"`
	Seq          int    `json:"seq"`
}

type GenerationEvent struct {
	MessageID        string          `json:"message_id"`
	Status           string          `json:"status,omitempty"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCallEvent `json:"tool_calls,omitempty"`
	Delta            string          `json:"delta,omitempty"`
	Error            string          `json:"error,omitempty"`
	ContentBytes     int             `json:"content_bytes,omitempty"`
	ReasoningBytes   int             `json:"reasoning_bytes,omitempty"`
}
