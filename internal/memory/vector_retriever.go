package memory

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/memory/embed"
	"github.com/AlexMeiko/guchat/internal/memory/vector"
)

var ErrInvalidRetrieverConfig = errors.New("invalid retriever config")

type VectorRetriever struct {
	fallback Retriever
	embedder embed.Embedder
	index    vector.Index
}

func NewVectorRetriever(fallback Retriever, embedder embed.Embedder, index vector.Index) (*VectorRetriever, error) {
	if fallback == nil || embedder == nil || index == nil {
		return nil, ErrInvalidRetrieverConfig
	}
	return &VectorRetriever{fallback: fallback, embedder: embedder, index: index}, nil
}

func (r *VectorRetriever) Search(ctx context.Context, input SearchInput) ([]SearchHit, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return r.fallback.Search(ctx, input)
	}

	result, err := r.embedder.EmbedTexts(ctx, embed.EmbedInput{Texts: []string{query}})
	if err != nil {
		log.Printf("memory vector search embed failed: error=%v", err)
		return r.fallback.Search(ctx, input)
	}
	if len(result.Vectors) != 1 || len(result.Vectors[0]) == 0 {
		return r.fallback.Search(ctx, input)
	}

	hits, err := r.index.Search(ctx, vector.SearchInput{
		Vector: result.Vectors[0],
		Limit:  normalizeLimit(input.Limit, 5, 20),
		Filter: buildVectorSearchFilter(input),
	})
	if err != nil {
		log.Printf("memory vector search failed: error=%v", err)
		return r.fallback.Search(ctx, input)
	}

	resultHits := make([]SearchHit, 0, len(hits))
	seen := map[int64]struct{}{}
	now := time.Now()

	for _, hit := range hits {
		searchHit, ok := vectorPayloadToSearchHit(hit.Payload)
		if !ok {
			continue
		}
		if _, exists := seen[searchHit.Item.ID]; exists {
			continue
		}
		if !isVectorHitVisible(searchHit.Item, input, now) {
			continue
		}

		seen[searchHit.Item.ID] = struct{}{}
		resultHits = append(resultHits, searchHit)
	}

	return resultHits, nil
}

func vectorPayloadToSearchHit(payload map[string]any) (SearchHit, bool) {
	id, ok := payloadInt64(payload, "memory_item_id")
	if !ok || id <= 0 {
		return SearchHit{}, false
	}

	content := payloadString(payload, "content")
	if strings.TrimSpace(content) == "" {
		return SearchHit{}, false
	}

	item := entity.MemoryItem{
		ID:        id,
		Scope:     payloadString(payload, "scope"),
		Category:  payloadString(payload, "category"),
		Status:    payloadString(payload, "status"),
		UpdatedAt: time.Time{},
	}

	if ownerID, ok := payloadInt64(payload, "owner_user_id"); ok {
		item.OwnerUserID = sql.NullInt64{Int64: ownerID, Valid: true}
	}
	if conversationID := payloadString(payload, "conversation_id"); conversationID != "" {
		item.ConversationID = sql.NullString{String: conversationID, Valid: true}
	}
	if expiresAt, ok := payloadTime(payload, "expires_at"); ok {
		item.ExpiresAt = sql.NullTime{Time: expiresAt, Valid: true}
	}
	if updatedAt, ok := payloadTime(payload, "updated_at"); ok {
		item.UpdatedAt = updatedAt
	}

	segmentIndex := 0
	if value, ok := payloadInt64(payload, "segment_index"); ok {
		segmentIndex = int(value)
	}

	return SearchHit{
		Item:         item,
		Content:      content,
		SegmentIndex: segmentIndex,
	}, true
}

func buildVectorSearchFilter(input SearchInput) *vector.Filter {
	filter := &vector.Filter{
		Must: []vector.Condition{
			{Key: "status", Value: MemoryStatusActive},
		},
		ShouldFilters: []vector.Filter{
			{
				Must: []vector.Condition{
					{Key: "scope", Value: MemoryScopeGlobal},
				},
			},
			{
				Must: []vector.Condition{
					{Key: "scope", Value: MemoryScopeUser},
					{Key: "owner_user_id", Value: input.UserID},
				},
			},
		},
	}

	conversationID := strings.TrimSpace(input.ConversationID)
	if conversationID != "" {
		filter.ShouldFilters = append(filter.ShouldFilters, vector.Filter{
			Must: []vector.Condition{
				{Key: "scope", Value: MemoryScopeConversation},
				{Key: "owner_user_id", Value: input.UserID},
				{Key: "conversation_id", Value: conversationID},
			},
		})
	}

	if len(input.Categories) == 1 {
		filter.Must = append(filter.Must, vector.Condition{
			Key:   "category",
			Value: input.Categories[0],
		})
	}

	if len(input.Scopes) == 1 {
		filter.Must = append(filter.Must, vector.Condition{
			Key:   "scope",
			Value: input.Scopes[0],
		})
	}

	return filter
}

func isVectorHitVisible(item entity.MemoryItem, input SearchInput, now time.Time) bool {
	if item.Status != MemoryStatusActive {
		return false
	}
	if item.ExpiresAt.Valid && !item.ExpiresAt.Time.After(now) {
		return false
	}
	if len(input.Categories) > 0 && !containsString(input.Categories, item.Category) {
		return false
	}
	if len(input.Scopes) > 0 && !containsString(input.Scopes, item.Scope) {
		return false
	}

	switch item.Scope {
	case MemoryScopeGlobal:
		return true
	case MemoryScopeUser:
		return item.OwnerUserID.Valid && item.OwnerUserID.Int64 == input.UserID
	case MemoryScopeConversation:
		return item.OwnerUserID.Valid &&
			item.OwnerUserID.Int64 == input.UserID &&
			item.ConversationID.Valid &&
			item.ConversationID.String == input.ConversationID
	default:
		return false
	}
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}

	text, ok := value.(string)
	if !ok {
		return ""
	}

	return text
}

func payloadInt64(payload map[string]any, key string) (int64, bool) {
	value, ok := payload[key]
	if !ok || value == nil {
		return 0, false
	}

	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		result := int64(typed)
		if typed < 0 || float64(result) != typed {
			return 0, false
		}
		return result, true
	default:
		return 0, false
	}
}

func payloadTime(payload map[string]any, key string) (time.Time, bool) {
	value := payloadString(payload, key)
	if value == "" {
		return time.Time{}, false
	}

	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}

	return parsed, true
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
