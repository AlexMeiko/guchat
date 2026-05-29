package entity

import (
	"database/sql"
	"time"
)

type MemoryItem struct {
	ID             int64          `db:"id"`
	OwnerUserID    sql.NullInt64  `db:"owner_user_id"`
	ConversationID sql.NullString `db:"conversation_id"`
	Scope          string         `db:"scope"`
	Category       string         `db:"category"`
	Origin         string         `db:"origin"`
	SourceType     string         `db:"source_type"`
	SourceRef      sql.NullString `db:"source_ref"`
	SourceTitle    sql.NullString `db:"source_title"`
	Content        string         `db:"content"`
	MetadataJSON   string         `db:"metadata_json"`
	Confidence     float64        `db:"confidence"`
	ExpiresAt      sql.NullTime   `db:"expires_at"`
	Status         string         `db:"status"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
}
