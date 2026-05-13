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
	"regexp"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/service"
)

type OpenAIChatCompletionsGenerator struct {
	client *http.Client
}

var toolCallTagRe = regexp.MustCompile(`<!--tool_call:([^>]+)-->`)

func NewOpenAIChatCompletionsGenerator(client *http.Client) *OpenAIChatCompletionsGenerator {
	if client == nil {
		client = http.DefaultClient
	}

	return &OpenAIChatCompletionsGenerator{
		client: client,
	}
}

func (g *OpenAIChatCompletionsGenerator) Generate(ctx context.Context, input service.GenerateInput, cb service.GenerateCallbacks) error {
	if input.Model == nil {
		return errors.New("model config is required")
	}

	apiKey := strings.TrimSpace(input.Model.APIKey)
	if apiKey == "" {
		return errors.New("model api key is required")
	}

	messages := buildOpenAIChatCompletionMessages(input.Messages)
	tools := buildOpenAIChatTools(input.Tools)

	if len(messages) == 0 {
		return errors.New("no prompt messages to send")
	}

	reqBody := openAIChatCompletionRequest{
		Model:    input.Model.ModelKey,
		Messages: messages,
		Tools:    tools,
		Stream:   true,
	}

	payload, err := marshalOpenAIRequestBody(
		reqBody,
		input.Model.ExtraBody,
		"model",
		"messages",
		"stream",
		"tools",
		"tool_choice",
	)
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

type openAIChatCompletionMessage struct {
	Role       string               `json:"role"`
	Content    string               `json:"content,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIChatToolCall `json:"tool_calls,omitempty"`
}

type openAIChatToolCall struct {
	// 并行 function calling 流式返回时不一定是先完整返回一个工具再返回一个的，可能为交错返回，所以需要Index
	Index    int    `json:"index,omitempty"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// 用于流式时拼接完整工具请求
type openAIChatToolCallAccumulator struct {
	ID        string
	Name      string
	Arguments string
}

// 发送给模型的工具定义
type openAIChatTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type openAIChatCompletionRequest struct {
	Model    string                        `json:"model"`
	Messages []openAIChatCompletionMessage `json:"messages"`
	Tools    []openAIChatTool              `json:"tools,omitempty"`
	Stream   bool                          `json:"stream"`
}

type openAIChatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content          string               `json:"content"`
			ReasoningContent string               `json:"reasoning_content"`
			ToolCalls        []openAIChatToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// 用于把模型返回的工具调用请求转换为符合的消息，在提交工具结果前，必须先把模型当时请求工具的 assistant tool_calls 消息放回上下文
func newOpenAIChatToolCallMessage(call service.ToolCall) openAIChatCompletionMessage {
	toolCall := openAIChatToolCall{
		ID:   call.ID,
		Type: "function",
	}
	toolCall.Function.Name = call.Name
	toolCall.Function.Arguments = string(call.Arguments)

	return openAIChatCompletionMessage{
		Role:      entity.MessageRoleAssistant,
		ToolCalls: []openAIChatToolCall{toolCall},
	}
}

// 用于构建 OpenAI Chat Completions 的 API URL，确保末尾是 /chat/completions
func buildOpenAIChatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}

	return baseURL + "/chat/completions"
}

// 将后端的工具定义转换为 Chat Completions 的 tools 参数
func buildOpenAIChatTools(tools []service.ToolDefinition) []openAIChatTool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]openAIChatTool, 0, len(tools))
	for _, tool := range tools {
		item := openAIChatTool{
			Type: "function",
		}
		item.Function.Name = tool.Name
		item.Function.Description = tool.Description
		item.Function.Parameters = tool.Parameters
		result = append(result, item)
	}

	return result
}

// 用于将项目的历史消息和已完成工具交换记录转换为 Chat Completions messages，
// 普通历史消息会变成 system/user/assistant 文本消息，每个 ToolExchange 会变成一条 assistant tool_calls 消息和一条 tool 结果消息。
func buildOpenAIChatCompletionMessages(
	messages []service.GenerateMessage,
) []openAIChatCompletionMessage {

	result := make([]openAIChatCompletionMessage, 0, len(messages)*2)

	for _, message := range messages {
		switch message.Role {
		case entity.MessageRoleSystem, entity.MessageRoleUser, entity.MessageRoleAssistant:
		default:
			continue
		}

		if message.Role == entity.MessageRoleAssistant && len(message.ToolExchanges) > 0 {
			result = appendOpenAIChatAssistantMessageWithTools(result, message)
			continue
		}

		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}

		result = append(result, openAIChatCompletionMessage{
			Role:    message.Role,
			Content: content,
		})
	}

	return result
}

func appendOpenAIChatAssistantMessageWithTools(
	result []openAIChatCompletionMessage,
	message service.GenerateMessage,
) []openAIChatCompletionMessage {
	exchanges := make(map[string]service.ToolExchange, len(message.ToolExchanges))
	for _, exchange := range message.ToolExchanges {
		exchanges[exchange.Call.ID] = exchange
	}
	content := message.Content
	matches := toolCallTagRe.FindAllStringSubmatchIndex(content, -1)

	last := 0
	for _, match := range matches {
		//fullStart/fullEnd 整个 <!--tool_call:call_a--> 的位置
		//idStart/idEnd     call_a 的位置
		fullStart := match[0]
		fullEnd := match[1]
		idStart := match[2]
		idEnd := match[3]

		text := strings.TrimSpace(content[last:fullStart])
		if text != "" {
			result = append(result, openAIChatCompletionMessage{
				Role:    entity.MessageRoleAssistant,
				Content: text,
			})
		}

		toolCallID := content[idStart:idEnd]
		exchange, ok := exchanges[toolCallID]
		if ok {
			result = append(result, newOpenAIChatToolCallMessage(exchange.Call))
			result = append(result, openAIChatCompletionMessage{
				Role:       "tool",
				ToolCallID: exchange.Call.ID,
				Content:    string(exchange.Result.Result),
			})
		}

		last = fullEnd
	}

	text := strings.TrimSpace(content[last:])
	if text != "" {
		result = append(result, openAIChatCompletionMessage{
			Role:    entity.MessageRoleAssistant,
			Content: text,
		})
	}

	return result
}

func streamOpenAIChatCompletion(body io.Reader, cb service.GenerateCallbacks) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	dataLines := make([]string, 0, 4)
	toolCalls := make([]openAIChatToolCallAccumulator, 0)
	toolCallsEmitted := false

	emitToolCalls := func() {
		if toolCallsEmitted {
			return
		}
		toolCallsEmitted = true

		if cb.ToolCallCreated == nil {
			return
		}

		for _, call := range toolCalls {
			if call.Name == "" {
				continue
			}

			cb.ToolCallCreated(service.ToolCall{
				ID:        call.ID,
				Name:      call.Name,
				Arguments: json.RawMessage(call.Arguments),
			})
		}
	}

	flushEvent := func() error {
		if len(dataLines) == 0 {
			return nil
		}

		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]

		if payload == "[DONE]" {
			emitToolCalls()
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

			for _, call := range choice.Delta.ToolCalls {
				for len(toolCalls) <= call.Index {
					toolCalls = append(toolCalls, openAIChatToolCallAccumulator{})
				}

				tmp := &toolCalls[call.Index]

				if call.ID != "" {
					tmp.ID = call.ID
				}
				if call.Function.Name != "" {
					tmp.Name = call.Function.Name
				}
				if call.Function.Arguments != "" {
					tmp.Arguments += call.Function.Arguments
				}
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

	emitToolCalls()
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
