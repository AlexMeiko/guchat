package service

import (
	"context"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
)

var ErrUnsupportedModelProvider = errors.New("unsupported model provider")

type GenerateInput struct {
	Model    *entity.ModelConfig
	Messages []entity.Message
}

type GenerateResult struct {
	Content          string
	ReasoningContent string
}

type Generator interface {
	Generate(ctx context.Context, input GenerateInput) (*GenerateResult, error)
}

type GeneratorFactory interface {
	Get(model *entity.ModelConfig) (Generator, error)
}
