package memory

import (
	"context"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/jmoiron/sqlx"
)

type MySQLStore struct {
	db *sqlx.DB
}

func NewMySQLStore(db *sqlx.DB) *MySQLStore {
	return &MySQLStore{db: db}
}

func (s *MySQLStore) Create(ctx context.Context, item *entity.MemoryItem) error {
	const query = `
INSERT INTO memory_items (
	    owner_user_id,
	    conversation_id,
	    scope,
	    category,
	    origin,
	    source_type,
	    source_ref,
	    source_title,
	    content,
	    metadata_json,
	    confidence,
	    expires_at,
	    status
) VALUES (
          :owner_user_id,
          :conversation_id,
          :scope,
          :category,
          :origin,
          :source_type,
          :source_ref,
          :source_title,
          :content,
          :metadata_json,
          :confidence,
          :expires_at,
          :status
)`

	result, err := s.db.NamedExecContext(ctx, query, item)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}

	item.ID = id
	return nil
}

func (s *MySQLStore) List(ctx context.Context, filter ListFilter) ([]entity.MemoryItem, error) {
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []string{MemoryStatusActive, MemoryStatusDisabled}
	}

	scopes := filter.Scopes
	if len(scopes) == 0 {
		scopes = []string{MemoryScopeUser, MemoryScopeConversation}
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	query := `
SELECT *
FROM memory_items
WHERE owner_user_id = ?
  AND scope IN (?)
  AND status IN (?)`

	args := []any{filter.UserID, scopes, statuses}

	if len(filter.Categories) > 0 {
		query += ` AND category IN (?)`
		args = append(args, filter.Categories)
	}

	query += `
ORDER BY updated_at DESC, id DESC
LIMIT ? OFFSET ?`
	args = append(args, normalizeLimit(filter.Limit, 50, 100), offset)

	query, finalArgs, err := sqlx.In(query, args...)
	if err != nil {
		return nil, err
	}

	query = s.db.Rebind(query)

	var items []entity.MemoryItem
	if err := s.db.SelectContext(ctx, &items, query, finalArgs...); err != nil {
		return nil, err
	}

	return items, nil
}

func (s *MySQLStore) GetByID(ctx context.Context, userID int64, id int64) (*entity.MemoryItem, error) {
	const query = `SELECT * FROM memory_items WHERE id = ? AND (scope = 'global' OR (scope IN ('user', 'conversation') AND owner_user_id = ?))`

	var memoryItem entity.MemoryItem
	if err := s.db.GetContext(ctx, &memoryItem, query, id, userID); err != nil {
		return nil, err
	}

	return &memoryItem, nil
}

func (s *MySQLStore) UpdateStatus(ctx context.Context, userID int64, id int64, status string) (bool, error) {
	const query = `UPDATE memory_items SET status = ? WHERE id = ? AND  owner_user_id = ? AND scope IN ('user', 'conversation')`

	result, err := s.db.ExecContext(ctx, query, status, id, userID)
	if err != nil {
		return false, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return rowsAffected > 0, nil
}

func (s *MySQLStore) SoftDelete(ctx context.Context, userID int64, id int64) (bool, error) {
	return s.UpdateStatus(ctx, userID, id, MemoryStatusDeleted)
}

func (s *MySQLStore) Search(ctx context.Context, input SearchInput) ([]entity.MemoryItem, error) {
	query := `SELECT * FROM memory_items WHERE status = 'active'
    AND (expires_at IS NULL OR expires_at > ?)
    AND (
    	scope = 'global'
    	OR (scope = 'user' AND owner_user_id = ?)
    	OR (scope = 'conversation' AND owner_user_id = ? AND conversation_id = ?)
)`
	args := []any{time.Now(), input.UserID, input.UserID, input.ConversationID}

	keywords := normalizeSearchKeywords(input.Keywords, 5)
	if len(keywords) == 0 {
		return []entity.MemoryItem{}, nil
	}

	likeParts := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		likeParts = append(likeParts, `content LIKE ? ESCAPE '\\'`)
		args = append(args, "%"+escapeLike(keyword)+"%")
	}
	query += ` AND (` + strings.Join(likeParts, " OR ") + `)`

	if len(input.Categories) > 0 {
		query += ` AND category IN (?)`
		args = append(args, input.Categories)
	}

	if len(input.Scopes) > 0 {
		query += ` AND scope IN (?)`
		args = append(args, input.Scopes)
	}

	query += ` ORDER BY
    	confidence DESC,
        updated_at DESC,
    	id DESC
    LIMIT ?`
	args = append(args, normalizeLimit(input.Limit, 5, 20))

	query, finalArgs, err := sqlx.In(query, args...)
	if err != nil {
		return nil, err
	}

	query = s.db.Rebind(query)

	var items []entity.MemoryItem
	if err := s.db.SelectContext(ctx, &items, query, finalArgs...); err != nil {
		return nil, err
	}

	return items, nil
}

func normalizeLimit(limit int, defaultLimit int, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func normalizeSearchKeywords(values []string, maxKeywords int) []string {
	if maxKeywords <= 0 {
		return nil
	}

	keywords := make([]string, 0, maxKeywords)
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		keyword := strings.TrimSpace(value)
		if keyword == "" {
			continue
		}
		if _, ok := seen[keyword]; ok {
			continue
		}

		seen[keyword] = struct{}{}
		keywords = append(keywords, keyword)
		if len(keywords) >= maxKeywords {
			break
		}
	}

	return keywords
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	)
	return replacer.Replace(value)
}
