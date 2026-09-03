package generator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/service"
)

type OpenAIResponsesGenerator struct {
	client *http.Client
}

type openAIResponsesMessage struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`

	Type      string `json:"type,omitempty"`
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Output    string `json:"output,omitempty"`
}

type openAIResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type openAIResponsesRequest struct {
	Model  string                   `json:"model"`
	Input  []openAIResponsesMessage `json:"input"`
	Tools  []json.RawMessage        `json:"tools,omitempty"`
	Stream bool                     `json:"stream"`
}

func NewOpenAIResponsesGenerator(client *http.Client) *OpenAIResponsesGenerator {
	if client == nil {
		client = http.DefaultClient
	}

	return &OpenAIResponsesGenerator{
		client: client,
	}
}

func (g *OpenAIResponsesGenerator) Generate(ctx context.Context, input service.GenerateInput, cb service.GenerateCallbacks) error {
	if input.Model == nil {
		return errors.New("model config is required")
	}

	apiKey := strings.TrimSpace(input.Model.APIKey)
	if apiKey == "" {
		return errors.New("model api key is required")
	}

	messages := buildOpenAIInputMessages(input.Messages)
	if len(messages) == 0 {
		return errors.New("no prompt messages to send")
	}

	tools, err := buildOpenAIResponsesTools(input.Tools)
	if err != nil {
		return err
	}

	extraTools, extraBody, err := takeOpenAIResponsesExtraTools(input.Model.ExtraBody)
	if err != nil {
		return err
	}
	tools = append(tools, extraTools...)

	reqBody := openAIResponsesRequest{
		Model:  input.Model.ModelKey,
		Input:  messages,
		Tools:  tools,
		Stream: true,
	}

	payload, err := marshalOpenAIRequestBody(
		reqBody,
		extraBody,
		"model",
		"input",
		"tools",
		"stream",
	)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		buildOpenAIResponsesURL(input.Model.BaseURL),
		bytes.NewBuffer(payload),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeOpenAIError(resp)
	}

	return streamOpenAIResponses(resp.Body, cb)
}

type openAIResponsesEvent struct {
	Type        string `json:"type"`
	Delta       string `json:"delta"`
	OutputIndex int    `json:"output_index"`
	Item        struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
}

func streamOpenAIResponses(body io.ReadCloser, cb service.GenerateCallbacks) error {
	dataLines := make([]string, 0, 4)
	var event string

	flushEvent := func() error {
		if len(dataLines) == 0 {
			event = ""
			return nil
		}

		rawData := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		currentEvent := event
		event = ""

		var payload openAIResponsesEvent
		if err := json.Unmarshal([]byte(rawData), &payload); err != nil {
			return err
		}

		if currentEvent == "" {
			currentEvent = payload.Type
		}

		switch currentEvent {
		case "response.output_text.delta":
			if payload.Delta != "" && cb.ContentDelta != nil {
				cb.ContentDelta(payload.Delta)
			}
		case "response.reasoning_summary_text.delta":
			if payload.Delta != "" && cb.ReasoningDelta != nil {
				cb.ReasoningDelta(payload.Delta)
			}
		case "response.output_item.done":
			if payload.Item.Type == "function_call" && cb.ToolCallCreated != nil {
				cb.ToolCallCreated(service.ToolCall{
					ID:        payload.Item.CallID,
					Name:      payload.Item.Name,
					Arguments: json.RawMessage(payload.Item.Arguments),
				})
			}
		case "response.completed":
			return errOpenAIStreamDone
		}

		return nil
	}

	if err := forEachSSELine(body, func(line string) error {
		switch {
		case line == "":
			return flushEvent()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
		return nil
	}); err != nil {
		if errors.Is(err, errOpenAIStreamDone) {
			return nil
		}
		return err
	}

	if err := flushEvent(); err != nil && !errors.Is(err, errOpenAIStreamDone) {
		return err
	}

	return nil

}

func buildOpenAIResponsesURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/responses") {
		return baseURL
	}

	return baseURL + "/responses"
}

func buildOpenAIResponsesTools(tools []service.ToolDefinition) ([]json.RawMessage, error) {
	if len(tools) == 0 {
		return nil, nil
	}
	result := make([]json.RawMessage, 0, len(tools))

	for _, tool := range tools {
		tmp := openAIResponsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}

		jsonData, err := json.Marshal(tmp)
		if err != nil {
			return nil, err
		}

		result = append(result, jsonData)
	}

	return result, nil
}

func buildOpenAIInputMessages(messages []service.GenerateMessage) []openAIResponsesMessage {

	result := make([]openAIResponsesMessage, 0, len(messages))

	for _, message := range messages {
		switch message.Role {
		case entity.MessageRoleSystem, entity.MessageRoleUser, entity.MessageRoleAssistant:
			if message.Role == entity.MessageRoleAssistant && len(message.GenerationRounds) > 0 {
				result = appendOpenAIResponsesAssistantMessageWithRounds(result, message)
				continue
			}

			if message.Role == entity.MessageRoleAssistant && len(message.ToolExchanges) > 0 {
				result = appendOpenAIResponsesAssistantMessageWithLegacyTools(result, message)
				continue
			}

		default:
			continue
		}

		content := strings.TrimSpace(message.Content)
		if message.Role == entity.MessageRoleAssistant {
			content = strings.TrimSpace(toolCallTagRe.ReplaceAllString(content, ""))
		}

		if content != "" {
			result = append(result, openAIResponsesMessage{
				Role:    message.Role,
				Content: content,
			})
		}
	}

	return result
}

func appendOpenAIResponsesAssistantMessageWithRounds(
	result []openAIResponsesMessage,
	message service.GenerateMessage,
) []openAIResponsesMessage {
	exchangesByRound := groupToolExchangesByRound(message.ToolExchanges)

	for _, round := range message.GenerationRounds {
		content := sliceByOffset(message.Content, round.ContentStartOffset, round.ContentEndOffset)
		content = strings.TrimSpace(content)
		if message.Role == entity.MessageRoleAssistant {
			content = strings.TrimSpace(toolCallTagRe.ReplaceAllString(content, ""))
		}

		exchanges := exchangesByRound[round.Round]

		if content != "" {
			result = append(result, openAIResponsesMessage{
				Role:    message.Role,
				Content: content,
			})
		}

		for _, ex := range exchanges {
			result = append(result, openAIResponsesMessage{
				Type:      "function_call",
				CallID:    ex.Call.ID,
				Name:      ex.Call.Name,
				Arguments: string(ex.Call.Arguments),
			})
			result = append(result, openAIResponsesMessage{
				Type:   "function_call_output",
				CallID: ex.Call.ID,
				Output: string(ex.Result.Result),
			})
		}
	}

	return result
}

func appendOpenAIResponsesAssistantMessageWithLegacyTools(
	result []openAIResponsesMessage,
	message service.GenerateMessage,
) []openAIResponsesMessage {
	content := strings.TrimSpace(toolCallTagRe.ReplaceAllString(message.Content, ""))
	if content != "" {
		result = append(result, openAIResponsesMessage{
			Role:    entity.MessageRoleAssistant,
			Content: content,
		})
	}

	for _, ex := range message.ToolExchanges {
		result = append(result, openAIResponsesMessage{
			Type:      "function_call",
			CallID:    ex.Call.ID,
			Name:      ex.Call.Name,
			Arguments: string(ex.Call.Arguments),
		})
		result = append(result, openAIResponsesMessage{
			Type:   "function_call_output",
			CallID: ex.Call.ID,
			Output: string(ex.Result.Result),
		})
	}

	return result
}

// takeOpenAIResponsesExtraTools 取出ExtraBody中的Tools,并移除相应字段
func takeOpenAIResponsesExtraTools(extraBody string) ([]json.RawMessage, string, error) {
	if strings.TrimSpace(extraBody) == "" {
		return nil, extraBody, nil
	}

	var extra map[string]json.RawMessage
	if err := json.Unmarshal([]byte(extraBody), &extra); err != nil {
		return nil, "", err
	}

	rawTools, ok := extra["tools"]
	if !ok {
		return nil, extraBody, nil
	}

	var tools []json.RawMessage
	if err := json.Unmarshal(rawTools, &tools); err != nil {
		return nil, "", errors.New("extra_body tools must be an array")
	}

	delete(extra, "tools")

	if len(extra) == 0 {
		return tools, "", nil
	}

	rest, err := json.Marshal(extra)
	if err != nil {
		return nil, "", err
	}

	return tools, string(rest), nil
}
