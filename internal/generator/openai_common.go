package generator

import (
	"errors"

	"github.com/AlexMeiko/guchat/internal/entity"
)

var errOpenAIStreamDone = errors.New("stream done")

type openAIInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func buildOpenAIInputMessages(messages []entity.Message) []openAIInputMessage {
	result := make([]openAIInputMessage, 0, len(messages))

	for _, message := range messages {
		if message.Content == "" {
			continue
		}

		switch message.Role {
		case entity.MessageRoleSystem, entity.MessageRoleUser, entity.MessageRoleAssistant:
		default:
			continue
		}

		result = append(result, openAIInputMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	return result
}
