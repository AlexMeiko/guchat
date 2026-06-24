package memory

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
)

type SearchHit struct {
	Item         entity.MemoryItem
	Content      string
	SegmentIndex int
}

type Retriever interface {
	Search(ctx context.Context, input SearchInput) ([]SearchHit, error)
}
