package memory

import "context"

type MySQLRetriever struct {
	store Store
}

func NewMySQLRetriever(store Store) *MySQLRetriever {
	return &MySQLRetriever{store: store}
}

func (r *MySQLRetriever) Search(ctx context.Context, input SearchInput) ([]SearchHit, error) {
	items, err := r.store.Search(ctx, input)
	if err != nil {
		return nil, err
	}

	hits := make([]SearchHit, 0, len(items))
	for _, item := range items {
		hits = append(hits, SearchHit{
			Item:         item,
			Content:      item.Content,
			SegmentIndex: 0,
		})
	}

	return hits, nil
}
