package repository

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type ToolCallRepository struct {
	db *sqlx.DB
}

func NewToolCallRepository(db *sqlx.DB) *ToolCallRepository {
	return &ToolCallRepository{db: db}
}

func (r *ToolCallRepository) Create(ctx context.Context, call *entity.ToolCall) error {
	if call.Status == "" {
		call.Status = entity.ToolCallStatusPending
	}

	const query = `
		INSERT INTO tool_calls (
			conversation_id,
			assistant_message_id,
			provider_call_id,
			tool_name,
			arguments_json,
			result_json,
			status,
			error_message,
			round,
			seq
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		call.ConversationID,
		call.AssistantMessageID,
		call.ProviderCallID,
		call.ToolName,
		call.ArgumentsJSON,
		call.ResultJSON,
		call.Status,
		call.ErrorMessage,
		call.Round,
		call.Seq,
	)
	if err != nil {
		return err
	}

	call.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	return nil
}

func (r *ToolCallRepository) ListByAssistantMessageID(ctx context.Context, assistantMessageID string) ([]entity.ToolCall, error) {
	const query = `SELECT * FROM tool_calls WHERE assistant_message_id = ? ORDER BY round, seq, id`

	var toolCalls []entity.ToolCall
	err := r.db.SelectContext(ctx, &toolCalls, query, assistantMessageID)
	return toolCalls, err
}

func (r *ToolCallRepository) MarkRunning(ctx context.Context, id int64) error {
	const query = `UPDATE tool_calls SET status = ?, started_at = NOW(3), error_message = '' WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, entity.ToolCallStatusRunning, id)
	return err
}

func (r *ToolCallRepository) MarkDone(ctx context.Context, id int64, resultJSON string) error {
	const query = `UPDATE tool_calls SET status = ?, result_json = ?, error_message = '', finished_at = NOW(3) WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, entity.ToolCallStatusDone, resultJSON, id)
	return err
}

func (r *ToolCallRepository) MarkFailed(ctx context.Context, id int64, resultJSON string, errorMessage string) error {
	const query = `UPDATE tool_calls SET status = ?, result_json = ?, error_message = ?, finished_at = NOW(3) WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, entity.ToolCallStatusFailed, resultJSON, errorMessage, id)
	return err
}
