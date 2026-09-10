package service

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/AlexMeiko/guchat/internal/entity"
)

type storedImageReader interface {
	OpenStoredImage(userID int64, conversationID, storeKey string) (*os.File, error)
}

type ContextAssembler struct {
	images storedImageReader
}

func NewContextAssembler(images storedImageReader) *ContextAssembler {
	return &ContextAssembler{images: images}
}

type AssembleSession struct {
	user   UserContext
	images storedImageReader
	cache  map[string]*GenerateImage
}

func (a *ContextAssembler) Session(user UserContext) *AssembleSession {
	session := &AssembleSession{
		user:  user,
		cache: make(map[string]*GenerateImage),
	}
	if a != nil {
		session.images = a.images
	}
	return session
}

func (a *ContextAssembler) BindHistory(
	messages []entity.Message,
	calls []entity.ToolCall,
	rounds []entity.GenerationRound,
) []GenerateMessage {
	out := newGenerateMessages(messages)
	exchanges := groupToolExchangesByMessageID(calls)
	byMessage := groupGenerationRoundsByMessageID(rounds)
	for i := range out {
		out[i].ToolExchanges = exchanges[out[i].ID]
		out[i].GenerationRounds = byMessage[out[i].ID]
	}
	return out
}

func (s *AssembleSession) View(ctx context.Context, messages []GenerateMessage) ([]GenerateMessage, error) {
	out := make([]GenerateMessage, len(messages))
	copy(out, messages)
	for i := range out {
		if len(out[i].ToolExchanges) == 0 {
			continue
		}
		exchanges := make([]ToolExchange, len(out[i].ToolExchanges))
		copy(exchanges, out[i].ToolExchanges)
		for j := range exchanges {
			viewed, err := s.ViewResult(ctx, exchanges[j].Result)
			if err != nil {
				return nil, err
			}
			exchanges[j].Result = viewed
		}
		out[i].ToolExchanges = exchanges
	}
	return out, nil
}

func (s *AssembleSession) ViewResult(ctx context.Context, result ToolResult) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}

	var ref toolImageRef
	if err := json.Unmarshal(result.Result, &ref); err != nil {
		return result, nil
	}
	if strings.TrimSpace(ref.StoreKey) == "" && strings.TrimSpace(ref.Analysis) == "" {
		return result, nil
	}

	out := result
	out.Image = nil

	if !ref.OK {
		return replaceToolResult(out, imageView{
			OK:    false,
			Error: firstNonEmpty(ref.Error, "image is unavailable"),
		})
	}

	if s == nil || !s.user.SupportsVision {
		analysis := strings.TrimSpace(ref.Analysis)
		if analysis == "" {
			return replaceToolResult(out, imageView{
				OK:    false,
				Error: "current model does not support vision",
			})
		}
		return replaceToolResult(out, imageView{OK: true, Analysis: analysis})
	}

	img := s.loadImage(strings.TrimSpace(ref.StoreKey), strings.TrimSpace(ref.StoredMIMEType))
	if img == nil {
		return replaceToolResult(out, imageView{
			OK:    false,
			Error: "stored image is unavailable",
		})
	}
	out.Image = img
	return replaceToolResult(out, imageView{OK: true})
}

func (a *ContextAssembler) Prefix(summary string, memory *GenerateMessage, rest []GenerateMessage) []GenerateMessage {
	out := make([]GenerateMessage, 0, len(rest)+2)
	if memory != nil {
		out = append(out, *memory)
	}
	if msg := newSummaryGenerateMessage(summary); msg != nil {
		out = append(out, *msg)
	}
	return append(out, rest...)
}

func (a *ContextAssembler) Round(history []GenerateMessage, current GenerateMessage) []GenerateMessage {
	if current.Content == "" && current.ReasoningContent == "" &&
		len(current.ToolExchanges) == 0 && len(current.GenerationRounds) == 0 {
		return history
	}
	out := make([]GenerateMessage, 0, len(history)+1)
	out = append(out, history...)
	return append(out, current)
}

func (a *ContextAssembler) Estimate(messages []GenerateMessage, tools []ToolDefinition) int {
	total := estimateToolDefinitionsTokens(tools)
	for _, message := range messages {
		total += estimateGenerateMessageTokens(message)
	}
	return total
}

type toolImageRef struct {
	OK             bool   `json:"ok"`
	Error          string `json:"error,omitempty"`
	StoreKey       string `json:"store_key,omitempty"`
	StoredMIMEType string `json:"stored_mime_type,omitempty"`
	Analysis       string `json:"analysis,omitempty"`
}

type imageView struct {
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
	Analysis string `json:"analysis,omitempty"`
}

func (s *AssembleSession) loadImage(storeKey, mimeType string) *GenerateImage {
	if storeKey == "" || s == nil || s.images == nil {
		return nil
	}
	if img, ok := s.cache[storeKey]; ok {
		return img
	}

	file, err := s.images.OpenStoredImage(s.user.UserID, s.user.ConversationID, storeKey)
	if err != nil {
		s.cache[storeKey] = nil
		return nil
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) == 0 {
		s.cache[storeKey] = nil
		return nil
	}
	if mimeType == "" {
		mimeType = "image/jpeg"
	}
	img := &GenerateImage{MIMEType: mimeType, Bytes: data}
	s.cache[storeKey] = img
	return img
}

func replaceToolResult(result ToolResult, view imageView) (ToolResult, error) {
	payload, err := json.Marshal(view)
	if err != nil {
		return ToolResult{}, err
	}
	result.Result = payload
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func newGenerateMessages(messages []entity.Message) []GenerateMessage {
	result := make([]GenerateMessage, 0, len(messages))
	for _, message := range messages {
		result = append(result, GenerateMessage{
			ID:               message.ID,
			Role:             message.Role,
			Content:          message.Content,
			ReasoningContent: message.ReasoningContent,
		})
	}
	return result
}

func groupToolExchangesByMessageID(records []entity.ToolCall) map[string][]ToolExchange {
	result := make(map[string][]ToolExchange)

	for _, record := range records {
		if record.Status != entity.ToolCallStatusDone && record.Status != entity.ToolCallStatusFailed {
			continue
		}

		result[record.AssistantMessageID] = append(result[record.AssistantMessageID], ToolExchange{
			Round: record.Round,
			Seq:   record.Seq,
			Call: ToolCall{
				ID:        record.ProviderCallID,
				Name:      record.ToolName,
				Arguments: json.RawMessage(record.ArgumentsJSON),
			},
			Result: ToolResult{
				ToolCallID: record.ProviderCallID,
				Name:       record.ToolName,
				Result:     json.RawMessage(record.ResultJSON),
			},
		})
	}

	return result
}

func groupGenerationRoundsByMessageID(records []entity.GenerationRound) map[string][]GenerationRound {
	result := make(map[string][]GenerationRound)

	for _, record := range records {
		result[record.AssistantMessageID] = append(result[record.AssistantMessageID], GenerationRound{
			Round:                record.Round,
			ContentStartOffset:   record.ContentStartOffset,
			ContentEndOffset:     record.ContentEndOffset,
			ReasoningStartOffset: record.ReasoningStartOffset,
			ReasoningEndOffset:   record.ReasoningEndOffset,
		})
	}

	return result
}
