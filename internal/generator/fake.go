package generator

import (
	"context"
	"time"

	"github.com/AlexMeiko/guchat/internal/service"
)

type FakeGenerator struct{}

func NewFakeGenerator() *FakeGenerator {
	return &FakeGenerator{}
}

func (g *FakeGenerator) Generate(ctx context.Context, input service.GenerateInput, cb service.GenerateCallbacks) error {
	parts := []string{"fake ", "assistant ", "test ", "reply"}

	for _, part := range parts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if cb.ContentDelta != nil {
			cb.ContentDelta(part)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil
}
