package memory

import (
	"context"

	"github.com/AlexMeiko/guchat/internal/entity"
)

type Indexer interface {
	// Index 为单条记忆重建检索索引。
	// 应先删除该 memory_item_id 已存在的旧索引，再重新切分、向量化并写入新的索引数据。
	Index(ctx context.Context, item entity.MemoryItem) error

	// Delete 删除单条记忆对应的检索索引。
	// 只删除 Qdrant 等辅助索引中的数据，不删除 MySQL memory_items。
	Delete(ctx context.Context, memoryItemID int64) error
}
