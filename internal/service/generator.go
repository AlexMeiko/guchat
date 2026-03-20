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

type GenerateCallbacks struct {
	ContentDelta   func(string)
	ReasoningDelta func(string)
}

type Generator interface {
	Generate(ctx context.Context, input GenerateInput, cb GenerateCallbacks) error
}

type GeneratorFactory interface {
	Get(model *entity.ModelConfig) (Generator, error)
}
