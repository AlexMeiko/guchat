package memory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/memory/embed"
	"github.com/AlexMeiko/guchat/internal/memory/segment"
	"github.com/AlexMeiko/guchat/internal/memory/vector"
)

var ErrInvalidIndexerConfig = errors.New("invalid indexer config")
var ErrInvalidIndexerInput = errors.New("invalid indexer input")

type VectorIndexer struct {
	splitter segment.Splitter
	embedder embed.Embedder
	index    vector.Index
}

func NewVectorIndexer(
	splitter segment.Splitter,
	embedder embed.Embedder,
	index vector.Index,
) (*VectorIndexer, error) {
	if splitter == nil || embedder == nil || index == nil {
		return nil, ErrInvalidIndexerConfig
	}

	return &VectorIndexer{
		splitter: splitter,
		embedder: embedder,
		index:    index,
	}, nil
}

func (i *VectorIndexer) Index(ctx context.Context, item entity.MemoryItem) error {
	if item.ID <= 0 || strings.TrimSpace(item.Content) == "" {
		return ErrInvalidIndexerInput
	}

	if err := i.index.DeleteByMemoryItemID(ctx, item.ID); err != nil {
		return err
	}

	// 文本切分
	segments, err := i.splitter.Split(ctx, segment.SplitInput{
		Modality:    segment.ModalityText,
		Content:     item.Content,
		SourceTitle: nullStringValue(item.SourceTitle),
	})
	if err != nil {
		return err
	}

	texts := make([]string, 0, len(segments))
	for _, item := range segments {
		texts = append(texts, item.Content)
	}

	// 向量化
	result, err := i.embedder.EmbedTexts(ctx, embed.EmbedInput{
		Texts: texts,
	})
	if err != nil {
		return err
	}

	if len(result.Vectors) != len(segments) {
		return ErrInvalidIndexerInput
	}

	points := make([]vector.Point, 0, len(segments))
	for idx, seg := range segments {
		if len(result.Vectors[idx]) == 0 {
			return ErrInvalidIndexerInput
		}

		points = append(points, vector.Point{
			Vector:  result.Vectors[idx],
			Payload: memoryIndexPayload(item, seg, result.Model),
		})
	}

	return i.index.Upsert(ctx, points)
}

func (i *VectorIndexer) Delete(ctx context.Context, memoryItemID int64) error {
	return i.index.DeleteByMemoryItemID(ctx, memoryItemID)
}

func nullStringValue(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func memoryIndexPayload(item entity.MemoryItem, seg segment.Segment, embeddingModel string) map[string]any {
	return map[string]any{
		"memory_item_id":   item.ID,
		"owner_user_id":    nullableInt64Payload(item.OwnerUserID),
		"conversation_id":  nullableStringPayload(item.ConversationID),
		"scope":            item.Scope,
		"category":         item.Category,
		"status":           item.Status,
		"confidence":       item.Confidence,
		"source_type":      item.SourceType,
		"source_ref":       nullableStringPayload(item.SourceRef),
		"source_title":     nullableStringPayload(item.SourceTitle),
		"expires_at":       nullableTimePayload(item.ExpiresAt),
		"updated_at":       item.UpdatedAt.Format(time.RFC3339Nano),
		"segment_index":    seg.Index,
		"modality":         seg.Modality,
		"content":          seg.Content,
		"splitter_version": seg.SplitterVersion,
		"embedding_model":  embeddingModel,
	}
}

func nullableStringPayload(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func nullableInt64Payload(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

func nullableTimePayload(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.Format(time.RFC3339Nano)
}
