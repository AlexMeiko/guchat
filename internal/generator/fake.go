package generator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AlexMeiko/guchat/internal/service"
	"github.com/AlexMeiko/guchat/internal/tool"
)

type FakeGenerator struct{}

func NewFakeGenerator() *FakeGenerator {
	return &FakeGenerator{}
}

func hasTool(tools []service.ToolDefinition, name string) bool {
	for _, item := range tools {
		if item.Name == name {
			return true
		}
	}
	return false
}

func emitParts(ctx context.Context, cb service.GenerateCallbacks, parts []string) error {
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

func (g *FakeGenerator) Generate(ctx context.Context, input service.GenerateInput, cb service.GenerateCallbacks) error {
	if len(input.Messages) > 0 {
		last := input.Messages[len(input.Messages)-1]
		if len(last.ToolExchanges) > 0 {
			exchange := last.ToolExchanges[len(last.ToolExchanges)-1]
			return emitParts(ctx, cb, []string{
				"fake tool result: ",
				string(exchange.Result.Result),
			})
		}
	}

	if hasTool(input.Tools, tool.ToolGetCurrentTime) {
		if cb.ToolCallCreated != nil {
			cb.ToolCallCreated(service.ToolCall{
				ID:        "fake_call_get_current_time_1",
				Name:      tool.ToolGetCurrentTime,
				Arguments: json.RawMessage(`{"timezone":"Asia/Shanghai"}`),
			})
		}
		return nil
	}

	return emitParts(ctx, cb, []string{"fake ", "assistant ", "test ", "reply"})
}
