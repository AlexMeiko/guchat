package service

import (
	"unicode/utf8"

	"github.com/AlexMeiko/guchat/internal/entity"
)

const (
	estimatedMessageOverhead        = 8
	estimatedToolDefinitionOverhead = 16
	estimatedToolExchangeOverhead   = 16
)

func estimateTextTokens(content string) int {
	var asciiCount int
	var nonASCIICount int

	for _, r := range content {
		if r < utf8.RuneSelf {
			asciiCount++
		} else {
			nonASCIICount++
		}
	}

	return (asciiCount+2)/3 + nonASCIICount
}

func estimateGenerateMessageTokens(message GenerateMessage) int {
	total := estimatedMessageOverhead
	total += estimateTextTokens(message.Role)
	total += estimateTextTokens(message.Content)

	if message.Role == entity.MessageRoleAssistant {
		total += estimateTextTokens(message.ReasoningContent)
	}

	for _, exchange := range message.ToolExchanges {
		total += estimatedToolExchangeOverhead
		total += estimateTextTokens(exchange.Call.ID)
		total += estimateTextTokens(exchange.Call.Name)
		total += estimateTextTokens(string(exchange.Call.Arguments))
		total += estimateTextTokens(exchange.Result.ToolCallID)
		total += estimateTextTokens(exchange.Result.Name)
		total += estimateTextTokens(string(exchange.Result.Result))
	}

	return total
}

func estimateToolDefinitionsTokens(tools []ToolDefinition) int {
	total := 0

	for _, tool := range tools {
		total += estimatedToolDefinitionOverhead
		total += estimateTextTokens(tool.Name)
		total += estimateTextTokens(tool.Description)
		total += estimateTextTokens(string(tool.Parameters))
	}

	return total
}
