package service

import (
	"context"
	"database/sql"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
)

var ErrModelNotFound = errors.New("model not found")

type CreateModelInput struct {
	Name      string
	Provider  string
	ModelKey  string
	BaseURL   string
	APIKey    string
	IsEnabled bool
}

type UpdateModelInput struct {
	Name      *string
	Provider  *string
	ModelKey  *string
	BaseURL   *string
	APIKey    *string
	IsEnabled *bool
}

type ModelService struct {
	modelRepo *repository.ModelRepository
}

func NewModelService(modelRepo *repository.ModelRepository) *ModelService {
	return &ModelService{
		modelRepo: modelRepo,
	}
}

func (s *ModelService) ListEnabled(ctx context.Context) ([]entity.ModelConfig, error) {
	return s.modelRepo.ListEnabled(ctx)
}

func (s *ModelService) Create(ctx context.Context, input CreateModelInput) (*entity.ModelConfig, error) {
	model := &entity.ModelConfig{
		Name:      input.Name,
		Provider:  input.Provider,
		ModelKey:  input.ModelKey,
		BaseURL:   input.BaseURL,
		APIKey:    input.APIKey,
		IsEnabled: input.IsEnabled,
	}

	if err := s.modelRepo.Create(ctx, model); err != nil {
		return nil, err
	}

	created, err := s.modelRepo.GetByID(ctx, model.ID)
	if err != nil {
		return nil, err
	}

	return created, nil
}

func (s *ModelService) GetByID(ctx context.Context, id int64) (*entity.ModelConfig, error) {
	model, err := s.modelRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrModelNotFound
		}

		return nil, err
	}

	return model, nil
}

func (s *ModelService) UpdateByID(ctx context.Context, id int64, input UpdateModelInput) (*entity.ModelConfig, error) {
	model, err := s.modelRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrModelNotFound
		}
		return nil, err
	}

	if input.Name != nil {
		model.Name = *input.Name
	}
	if input.Provider != nil {
		model.Provider = *input.Provider
	}
	if input.ModelKey != nil {
		model.ModelKey = *input.ModelKey
	}
	if input.BaseURL != nil {
		model.BaseURL = *input.BaseURL
	}
	if input.APIKey != nil {
		model.APIKey = *input.APIKey
	}
	if input.IsEnabled != nil {
		model.IsEnabled = *input.IsEnabled
	}

	updated, err := s.modelRepo.UpdateByID(ctx, model)
	if err != nil {
		return nil, err
	}

	if !updated {
		return nil, ErrModelNotFound
	}

	result, err := s.modelRepo.GetByID(ctx, model.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrModelNotFound
		}
		return nil, err
	}

	return result, nil
}

func (s *ModelService) DeleteByID(ctx context.Context, id int64) error {
	deleted, err := s.modelRepo.DeleteByID(ctx, id)
	if err != nil {
		return err
	}

	if !deleted {
		return ErrModelNotFound
	}

	return nil
}
