package vector

import "context"

type Point struct {
	Vector  []float32
	Payload map[string]any
}

type SearchInput struct {
	Vector []float32
	Limit  int
	Filter map[string]any
}

type SearchHit struct {
	Payload map[string]any
}

type Index interface {
	// Upsert 写入向量点。
	// 具体实现负责生成或维护底层向量库所需的 point id。
	Upsert(ctx context.Context, points []Point) error

	// Search 根据向量检索相似点。
	// Filter 用于传递 owner_user_id、scope、category、status 等索引侧过滤条件。
	Search(ctx context.Context, input SearchInput) ([]SearchHit, error)

	// DeleteByMemoryItemID 删除某条记忆对应的全部向量点。
	// 第一版可通过 payload.memory_item_id 过滤删除；Qdrant 需要为 memory_item_id 创建 payload index。
	DeleteByMemoryItemID(ctx context.Context, memoryItemID int64) error
}
