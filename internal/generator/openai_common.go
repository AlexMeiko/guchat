package generator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

func marshalOpenAIRequestBody(base any, extraBody string, reservedKeys ...string) ([]byte, error) {
	basePayload, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(extraBody) == "" {
		return basePayload, nil
	}

	var body map[string]any
	if err := json.Unmarshal(basePayload, &body); err != nil {
		return nil, err
	}

	var extra map[string]any
	if err := json.Unmarshal([]byte(extraBody), &extra); err != nil {
		return nil, err
	}

	reserved := make(map[string]struct{}, len(reservedKeys))
	for _, key := range reservedKeys {
		reserved[key] = struct{}{}
	}

	for key, value := range extra {
		if _, ok := reserved[key]; ok {
			return nil, fmt.Errorf("extra_body contains reserved key: %s", key)
		}
		body[key] = value
	}

	return json.Marshal(body)
}
