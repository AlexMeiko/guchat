package generator

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/service"
)

type FakeGenerator struct{}

func NewFakeGenerator() *FakeGenerator {
	return &FakeGenerator{}
}

func (g *FakeGenerator) Generate(ctx context.Context, input service.GenerateInput) (*service.GenerateResult, error) {
	return &service.GenerateResult{
		Content:          "fake assistant reply",
		ReasoningContent: "",
	}, nil
}
