package repository

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *entity.User) error {
	const query = `INSERT INTO users (username, password_hash, role) VALUES (?, ?, ?)`

	result, err := r.db.ExecContext(ctx, query, user.Username, user.PasswordHash, user.Role)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	user.ID = id
	return nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*entity.User, error) {
	const query = `SELECT * FROM users WHERE username = ? LIMIT 1`

	var user entity.User
	if err := r.db.GetContext(ctx, &user, query, username); err != nil {
		return nil, err
	}

	return &user, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id int64) (*entity.User, error) {
	const query = `SELECT * FROM users WHERE id = ? LIMIT 1`
	var user entity.User
	if err := r.db.GetContext(ctx, &user, query, id); err != nil {
		return nil, err
	}

	return &user, nil
}
