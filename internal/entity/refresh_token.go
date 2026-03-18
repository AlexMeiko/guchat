package entity

import "time"

type RefreshToken struct {
	ID        int64      `db:"id"`
	JTI       string     `db:"jti"`
	UserID    int64      `db:"user_id"`
	ExpiresAt time.Time  `db:"expires_at"`
	RevokedAt *time.Time `db:"revoked_at"`
	CreatedAt time.Time  `db:"created_at"`
}
