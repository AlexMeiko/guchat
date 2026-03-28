package repository

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type ModelRepository struct {
	db *sqlx.DB
}

func NewModelRepository(db *sqlx.DB) *ModelRepository {
	return &ModelRepository{db: db}
}

func (r *ModelRepository) Create(ctx context.Context, model *entity.ModelConfig) error {
	const query = `
		INSERT INTO models (
			name,
			provider,
			model_key,
			base_url,
			api_key,
		    extra_body,
			is_enabled
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		model.Name,
		model.Provider,
		model.ModelKey,
		model.BaseURL,
		model.APIKey,
		model.ExtraBody,
		model.IsEnabled,
	)

	if err != nil {
		return err
	}

	model.ID, err = result.LastInsertId()
	return err
}

func (r *ModelRepository) ListEnabled(ctx context.Context) ([]entity.ModelConfig, error) {
	const query = `SELECT * FROM models WHERE is_enabled = TRUE ORDER BY id ASC`

	var models []entity.ModelConfig
	if err := r.db.SelectContext(ctx, &models, query); err != nil {
		return nil, err
	}

	return models, nil
}

func (r *ModelRepository) ListAll(ctx context.Context) ([]entity.ModelConfig, error) {
	const query = `SELECT * FROM models ORDER BY id ASC`
	var models []entity.ModelConfig
	if err := r.db.SelectContext(ctx, &models, query); err != nil {
		return nil, err
	}

	return models, nil
}

func (r *ModelRepository) GetByID(ctx context.Context, id int64) (*entity.ModelConfig, error) {
	const query = `SELECT * FROM models WHERE id = ? LIMIT 1`

	var model entity.ModelConfig
	if err := r.db.GetContext(ctx, &model, query, id); err != nil {
		return nil, err
	}

	return &model, nil
}

func (r *ModelRepository) UpdateByID(ctx context.Context, model *entity.ModelConfig) (bool, error) {
	const query = `
		UPDATE models
		SET
			name = ?,
			provider = ?,
			model_key = ?,
			base_url = ?,
			api_key = ?,
			extra_body = ?,
			is_enabled = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(
		ctx,
		query,
		model.Name,
		model.Provider,
		model.ModelKey,
		model.BaseURL,
		model.APIKey,
		model.ExtraBody,
		model.IsEnabled,
		model.ID,
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

func (r *ModelRepository) DeleteByID(ctx context.Context, id int64) (bool, error) {
	const query = `DELETE FROM models WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return false, err
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return n > 0, nil
}
