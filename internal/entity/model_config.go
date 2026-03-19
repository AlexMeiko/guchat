package entity

import "time"

type ModelConfig struct {
	ID        int64     `db:"id"`
	Name      string    `db:"name"`
	Provider  string    `db:"provider"`
	ModelKey  string    `db:"model_key"`
	BaseURL   string    `db:"base_url"`
	APIKey    string    `db:"api_key"`
	IsEnabled bool      `db:"is_enabled"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
