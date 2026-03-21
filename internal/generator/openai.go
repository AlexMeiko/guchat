package generator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/AlexMeiko/guchat/internal/service"
)

type OpenAIGenerator struct {
	client *http.Client
}

func NewOpenAIGenerator(client *http.Client) *OpenAIGenerator {
	if client == nil {
		client = http.DefaultClient
	}

	return &OpenAIGenerator{
		client: client,
	}
}

func (g *OpenAIGenerator) Generate(ctx context.Context, input service.GenerateInput, cb service.GenerateCallbacks) error {
	if input.Model == nil {
		return errors.New("model config is required")
	}

	apiKey := strings.TrimSpace(input.Model.APIKey)
	if apiKey == "" {
		return errors.New("model api key is required")
	}

	messages := buildOpenAIChatMessages(input.Messages)
	if len(messages) == 0 {
		return errors.New("no prompt messages to send")
	}

	reqBody := openAIChatCompletionRequest{
		Model:    input.Model.ModelKey,
		Messages: messages,
		Stream:   true,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"POST",
		buildOpenAIChatCompletionsURL(input.Model.BaseURL),
		bytes.NewReader(payload),
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

	return streamOpenAIChatCompletion(resp.Body, cb)
}

type openAIChatCompletionRequest struct {
	Model    string              `json:"model"`
	Messages []openAIChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type openAIChatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"delta"`
	} `json:"choices"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func buildOpenAIChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}

	return baseURL + "/chat/completions"
}

func streamOpenAIChatCompletion(body io.Reader, cb service.GenerateCallbacks) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	dataLines := make([]string, 0, 4)

	flushEvent := func() error {
		if len(dataLines) == 0 {
			return nil
		}

		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]

		if payload == "[DONE]" {
			return errOpenAIStreamDone
		}

		var chunk openAIChatCompletionChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return err
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" && cb.ContentDelta != nil {
				cb.ContentDelta(choice.Delta.Content)
			}

			if choice.Delta.ReasoningContent != "" && cb.ReasoningDelta != nil {
				cb.ReasoningDelta(choice.Delta.ReasoningContent)
			}
		}

		return nil
	}

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		if line == "" {
			if err := flushEvent(); err != nil {
				if errors.Is(err, errOpenAIStreamDone) {
					return nil
				}
				return err
			}
			continue
		}

		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
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

func decodeOpenAIError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("openai api error: status %d", resp.StatusCode)
	}

	var parsed openAIErrorResponse
	if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error.Message != "" {
		return fmt.Errorf("openai api error: status %d: %s", resp.StatusCode, parsed.Error.Message)
	}

	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("openai api error: status %d", resp.StatusCode)
	}

	return fmt.Errorf("openai api error: status %d: %s", resp.StatusCode, text)
}
