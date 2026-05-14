package service

import (
	"context"
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
)

var ErrUnsupportedModelProvider = errors.New("unsupported model provider")

type GenerateMessage struct {
	ID               string
	Role             string
	Content          string
	ReasoningContent string
	ToolExchanges    []ToolExchange
}

type GenerateInput struct {
	Model    *entity.ModelConfig
	Messages []GenerateMessage
	Tools    []ToolDefinition
}

type GenerateCallbacks struct {
	ContentDelta    func(string)
	ReasoningDelta  func(string)
	ToolCallCreated func(ToolCall)
}

type Generator interface {
	Generate(ctx context.Context, input GenerateInput, cb GenerateCallbacks) error
}

type GeneratorFactory interface {
	Get(model *entity.ModelConfig) (Generator, error)
}
