package repository

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type GenerationRoundRepository struct {
	db *sqlx.DB
}

func NewGenerationRoundRepository(db *sqlx.DB) *GenerationRoundRepository {
	return &GenerationRoundRepository{
		db: db,
	}
}

func (r *GenerationRoundRepository) Create(ctx context.Context, round *entity.GenerationRound) error {
	const query = `
		INSERT INTO generation_rounds (
			conversation_id,
			assistant_message_id,
			round,
			content_start_offset,
			content_end_offset,
			reasoning_start_offset,
			reasoning_end_offset
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		round.ConversationID,
		round.AssistantMessageID,
		round.Round,
		round.ContentStartOffset,
		round.ContentEndOffset,
		round.ReasoningStartOffset,
		round.ReasoningEndOffset,
	)
	if err != nil {
		return err
	}

	round.ID, err = result.LastInsertId()
	return err
}

func (r *GenerationRoundRepository) ListByAssistantMessageIDs(ctx context.Context, assistantMessageIDs []string) ([]entity.GenerationRound, error) {
	if len(assistantMessageIDs) == 0 {
		return nil, nil
	}

	query, args, err := sqlx.In(`
		SELECT *
		FROM generation_rounds
		WHERE assistant_message_id IN (?)
		ORDER BY assistant_message_id, round
	`, assistantMessageIDs)
	if err != nil {
		return nil, err
	}

	query = r.db.Rebind(query)

	var rounds []entity.GenerationRound
	err = r.db.SelectContext(ctx, &rounds, query, args...)
	return rounds, err
}
