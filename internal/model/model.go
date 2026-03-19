package model

import "time"

type CreateModelRequest struct {
	Name      string `json:"name" binding:"required"`
	Provider  string `json:"provider" binding:"required"`
	ModelKey  string `json:"model_key" binding:"required"`
	BaseURL   string `json:"base_url" binding:"required"`
	APIKey    string `json:"api_key" binding:"required"`
	IsEnabled bool   `json:"is_enabled"`
}

type UpdateModelRequest struct {
	Name      *string `json:"name"`
	Provider  *string `json:"provider"`
	ModelKey  *string `json:"model_key"`
	BaseURL   *string `json:"base_url"`
	APIKey    *string `json:"api_key"`
	IsEnabled *bool   `json:"is_enabled"`
}

type ModelDetailResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	ModelKey  string    `json:"model_key"`
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"api_key"`
	IsEnabled bool      `json:"is_enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ModelSimpleResponse struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}
