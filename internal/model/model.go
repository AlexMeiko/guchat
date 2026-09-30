package model

import (
	"encoding/json"
	"time"
)

type CreateModelRequest struct {
	Name           string          `json:"name" binding:"required"`
	Provider       string          `json:"provider" binding:"required"`
	ModelKey       string          `json:"model_key" binding:"required"`
	BaseURL        string          `json:"base_url" binding:"required"`
	APIKey         string          `json:"api_key" binding:"required"`
	ExtraBody      json.RawMessage `json:"extra_body"`
	IsEnabled      bool            `json:"is_enabled"`
	SupportsVision bool            `json:"supports_vision"`
}

type UpdateModelRequest struct {
	Name           *string          `json:"name"`
	Provider       *string          `json:"provider"`
	ModelKey       *string          `json:"model_key"`
	BaseURL        *string          `json:"base_url"`
	APIKey         *string          `json:"api_key"`
	ExtraBody      *json.RawMessage `json:"extra_body"`
	IsEnabled      *bool            `json:"is_enabled"`
	SupportsVision *bool            `json:"supports_vision"`
}

type ModelDetailResponse struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Provider       string          `json:"provider"`
	ModelKey       string          `json:"model_key"`
	BaseURL        string          `json:"base_url"`
	APIKey         string          `json:"api_key"`
	ExtraBody      json.RawMessage `json:"extra_body"`
	IsEnabled      bool            `json:"is_enabled"`
	SupportsVision bool            `json:"supports_vision"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type ModelSimpleResponse struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	SupportsVision bool   `json:"supports_vision"`
}
