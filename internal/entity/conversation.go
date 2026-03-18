package entity

import "time"

type Conversation struct {
	ID        string    `db:"id"`
	UserID    int64     `db:"user_id"`
	Title     string    `db:"title"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}
