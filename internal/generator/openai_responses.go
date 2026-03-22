package generator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/AlexMeiko/guchat/internal/service"
)

type OpenAIResponsesGenerator struct {
	client *http.Client
}

type openAIResponsesRequest struct {
	Model  string               `json:"model"`
	Input  []openAIInputMessage `json:"input"`
	Stream bool                 `json:"stream"`
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

	messages := buildOpenAIInputMessages(input.Messages)
	if len(messages) == 0 {
		return errors.New("no prompt messages to send")
	}

	reqBody := openAIResponsesRequest{
		Model:  input.Model.ModelKey,
		Input:  messages,
		Stream: true,
	}

	payload, err := json.Marshal(reqBody)
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
	Type  string `json:"type"`
	Delta string `json:"delta"`
}

func streamOpenAIResponses(body io.ReadCloser, cb service.GenerateCallbacks) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

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
			cb.ContentDelta(payload.Delta)
		case "response.reasoning_summary_text.delta":
			cb.ReasoningDelta(payload.Delta)
		case "response.completed":
			return errOpenAIStreamDone
		}

		return nil
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		switch {
		case line == "":
			if err := flushEvent(); err != nil {
				if errors.Is(err, errOpenAIStreamDone) {
					return nil
				}
				return err
			}
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimPrefix(line, "event:")
			event = strings.TrimSpace(event)
		case strings.HasPrefix(line, "data:"):
			dataLines = append(dataLines, strings.TrimPrefix(line, "data:"))
		}
	}

	if err := scanner.Err(); err != nil {
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
