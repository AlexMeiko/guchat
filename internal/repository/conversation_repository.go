package repository

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type ConversationRepository struct {
	db *sqlx.DB
}

func NewConversationRepository(db *sqlx.DB) *ConversationRepository {
	return &ConversationRepository{
		db: db,
	}
}

func (r *ConversationRepository) Create(ctx context.Context, conversation *entity.Conversation) error {
	const query = `INSERT INTO conversations (id, user_id, title) VALUES (?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, conversation.ID, conversation.UserID, conversation.Title)
	return err
}

func (r *ConversationRepository) ListByUserID(ctx context.Context, userID int64) ([]entity.Conversation, error) {
	const query = `SELECT * FROM conversations WHERE user_id = ? ORDER BY updated_at DESC, id DESC`

	var conversations []entity.Conversation
	if err := r.db.SelectContext(ctx, &conversations, query, userID); err != nil {
		return nil, err
	}

	return conversations, nil
}

func (r *ConversationRepository) GetByIDAndUserID(ctx context.Context, conversationID string, userID int64) (*entity.Conversation, error) {
	const query = `SELECT * FROM conversations WHERE id = ? AND user_id = ?`

	var conver entity.Conversation
	if err := r.db.GetContext(ctx, &conver, query, conversationID, userID); err != nil {
		return nil, err
	}

	return &conver, nil
}

func (r *ConversationRepository) UpdateTitleByIDAndUserID(ctx context.Context, conversationID string, userID int64, title string) (bool, error) {
	const query = `UPDATE conversations SET title = ? WHERE id = ? AND user_id = ?`

	result, err := r.db.ExecContext(ctx, query, title, conversationID, userID)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

func (r *ConversationRepository) DeleteByIDAndUserID(ctx context.Context, conversationID string, userID int64) (bool, error) {
	const query = `DELETE FROM conversations WHERE id = ? AND user_id = ?`
	result, err := r.db.ExecContext(ctx, query, conversationID, userID)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

func (r *ConversationRepository) TouchByIDAndUserID(ctx context.Context, conversationID string, userID int64) error {
	const query = `UPDATE conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?`

	_, err := r.db.ExecContext(ctx, query, conversationID, userID)
	return err
}
