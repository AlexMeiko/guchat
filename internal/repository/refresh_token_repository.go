package repository

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type RefreshTokenRepository struct {
	db *sqlx.DB
}

func NewRefreshTokenRepository(db *sqlx.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{
		db: db,
	}
}

func (r *RefreshTokenRepository) Create(ctx context.Context, token *entity.RefreshToken) error {
	const query = `INSERT INTO refresh_tokens (jti, user_id, expires_at) VALUES (?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query, token.JTI, token.UserID, token.ExpiresAt)
	if err != nil {
		return err
	}

	token.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	return nil
}

func (r *RefreshTokenRepository) GetByJTI(ctx context.Context, jti string) (*entity.RefreshToken, error) {
	const query = "SELECT * FROM refresh_tokens WHERE jti = ? LIMIT 1"

	var token entity.RefreshToken
	if err := r.db.GetContext(ctx, &token, query, jti); err != nil {
		return nil, err
	}

	return &token, nil
}

func (r *RefreshTokenRepository) GetByJTIAndUserID(ctx context.Context, jti string, userID int64) (*entity.RefreshToken, error) {
	const query = `SELECT * FROM refresh_tokens WHERE jti = ? AND user_id = ? LIMIT 1`

	var token entity.RefreshToken
	if err := r.db.GetContext(ctx, &token, query, jti, userID); err != nil {
		return nil, err
	}

	return &token, nil
}

func (r *RefreshTokenRepository) RevokeByJTIAndUserID(ctx context.Context, jti string, userID int64) (bool, error) {
	const query = `UPDATE refresh_tokens SET revoked_at = NOW() WHERE jti = ? AND user_id = ? AND revoked_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, jti, userID)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}
