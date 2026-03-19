package model

type CreateGenerationRequest struct {
	ModelID int64 `json:"model_id" binding:"required"`
}
