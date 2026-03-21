package repository

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type MessageRepository struct {
	db *sqlx.DB
}

func NewMessageRepository(db *sqlx.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) Create(ctx context.Context, message *entity.Message) error {
	const query = `
		INSERT INTO messages (
            id,
            conversation_id,
            role,
            content,
            reasoning_content,
            status,
            error_message,
            seq
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		message.ID,
		message.ConversationID,
		message.Role,
		message.Content,
		message.ReasoningContent,
		message.Status,
		message.ErrorMessage,
		message.Seq,
	)
	return err
}

func (r *MessageRepository) ListPageByConversationID(
	ctx context.Context,
	conversationID string,
	beforeSeq *int,
	limit int,
) ([]entity.Message, bool, error) {
	queryLimit := limit + 1
	var messages []entity.Message
	var err error

	if beforeSeq == nil {
		const query = `SELECT * FROM messages WHERE conversation_id = ? ORDER BY seq DESC LIMIT ?`

		err = r.db.SelectContext(ctx, &messages, query, conversationID, queryLimit)
	} else {
		const query = `SELECT * FROM messages WHERE conversation_id = ? AND seq < ? ORDER BY seq DESC LIMIT ?`

		err = r.db.SelectContext(ctx, &messages, query, conversationID, *beforeSeq, queryLimit)
	}

	if err != nil {
		return nil, false, err
	}

	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}

	reverseMessages(messages)
	return messages, hasMore, nil
}

func (r *MessageRepository) ListByConversationIDBeforeOrEqualSeq(
	ctx context.Context,
	conversationID string,
	seq int,
) ([]entity.Message, error) {
	const query = `SELECT * FROM messages WHERE conversation_id = ? AND seq <= ? ORDER BY seq ASC`

	var messages []entity.Message
	if err := r.db.SelectContext(ctx, &messages, query, conversationID, seq); err != nil {
		return nil, err
	}

	return messages, nil
}

func (r *MessageRepository) GetByID(ctx context.Context, messageID string) (*entity.Message, error) {
	const query = `SELECT * FROM messages WHERE id = ?`

	var message entity.Message
	if err := r.db.GetContext(ctx, &message, query, messageID); err != nil {
		return nil, err
	}

	return &message, nil
}

func (r *MessageRepository) GetByIDAndConversationID(ctx context.Context, messageID, conversationID string) (*entity.Message, error) {
	const query = `SELECT * FROM messages WHERE id = ? AND conversation_id = ?`

	var message entity.Message
	if err := r.db.GetContext(ctx, &message, query, messageID, conversationID); err != nil {
		return nil, err
	}

	return &message, nil
}

func (r *MessageRepository) GetLastSeqByConversationID(ctx context.Context, conversationID string) (int, error) {
	const query = `SELECT COALESCE(MAX(seq), 0) FROM messages WHERE conversation_id = ?`

	var seq int
	if err := r.db.GetContext(ctx, &seq, query, conversationID); err != nil {
		return 0, err
	}
	return seq, nil
}

func (r *MessageRepository) GetNextSeqByConversationIDAndSeq(ctx context.Context, conversationID string, seq int) (int, bool, error) {
	const query = `SELECT seq FROM messages WHERE conversation_id = ? AND seq > ? ORDER BY seq ASC LIMIT 1`

	if err := r.db.GetContext(ctx, &seq, query, conversationID, seq); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}

	return seq, true, nil
}

func (r *MessageRepository) UpdateContentByIDAndConversationID(ctx context.Context, messageID, conversationID, content string) (bool, error) {
	const query = `UPDATE messages SET content = ? WHERE id = ? AND conversation_id = ?`

	result, err := r.db.ExecContext(ctx, query, content, messageID, conversationID)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

func (r *MessageRepository) DeleteByIDAndConversationID(ctx context.Context, messageID, conversationID string) (bool, error) {
	const query = `DELETE FROM messages WHERE id = ? AND conversation_id = ?`
	result, err := r.db.ExecContext(ctx, query, messageID, conversationID)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

func (r *MessageRepository) UpdateByIDAndConversationID(ctx context.Context, message *entity.Message) (bool, error) {
	const query = `
		UPDATE messages
		SET
			content = ?,
			reasoning_content = ?,
			status = ?,
			error_message = ?
		WHERE id = ? AND conversation_id = ?
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		message.Content,
		message.ReasoningContent,
		message.Status,
		message.ErrorMessage,
		message.ID,
		message.ConversationID,
	)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}

func (r *MessageRepository) FailUnfinishedMsg(ctx context.Context, errorMessage string) (int64, error) {
	const query = `UPDATE messages SET status = ?, error_message = ? WHERE role = ? AND status IN (?, ?)`

	result, err := r.db.ExecContext(
		ctx,
		query,
		entity.MessageStatusFailed,
		errorMessage,
		entity.MessageRoleAssistant,
		entity.MessageStatusPending,
		entity.MessageStatusStreaming,
	)
	if err != nil {
		return 0, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return n, nil
}

func reverseMessages(messages []entity.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}
