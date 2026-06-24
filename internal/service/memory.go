package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/memory"
	"github.com/AlexMeiko/guchat/internal/repository"
)

var ErrMemoryItemNotFound = errors.New("memory item not found")
var ErrInvalidMemoryInput = errors.New("invalid memory input")

var (
	allowedPrivateMemoryScopes = []string{
		memory.MemoryScopeUser,
		memory.MemoryScopeConversation,
	}
	allowedSearchMemoryScopes = []string{
		memory.MemoryScopeUser,
		memory.MemoryScopeConversation,
		memory.MemoryScopeGlobal,
	}
	allowedMemoryStatuses = []string{
		memory.MemoryStatusActive,
		memory.MemoryStatusDisabled,
		memory.MemoryStatusDeleted,
	}
	allowedMemoryCategories = []string{
		memory.MemoryCategoryUserProfile,
		memory.MemoryCategoryPreference,
		memory.MemoryCategoryFact,
		memory.MemoryCategoryKnowledge,
		memory.MemoryCategoryGoal,
		memory.MemoryCategoryRelationship,
		memory.MemoryCategoryExperience,
		memory.MemoryCategoryDailySummary,
		memory.MemoryCategoryConstraint,
		memory.MemoryCategoryNegativePreference,
		memory.MemoryCategorySituational,
	}
	allowedMemoryOrigins = []string{
		memory.MemoryOriginUserExplicit,
		memory.MemoryOriginUserImported,
		memory.MemoryOriginBehaviorInferred,
		memory.MemoryOriginAssistantSummary,
		memory.MemoryOriginSystemGenerated,
		memory.MemoryOriginToolGenerated,
	}
	allowedMemorySourceTypes = []string{
		memory.MemorySourceTypeNone,
		memory.MemorySourceTypeConversation,
		memory.MemorySourceTypeWeb,
		memory.MemorySourceTypeFile,
		memory.MemorySourceTypeAPI,
		memory.MemorySourceTypeRepo,
		memory.MemorySourceTypeManual,
	}
)

type MemoryService struct {
	memoryStore      memory.Store
	memoryRetriever  memory.Retriever
	conversationRepo *repository.ConversationRepository
}

type CreateMemoryInput struct {
	ConversationID string
	Scope          string
	Category       string
	Origin         string
	SourceType     string
	SourceRef      string
	SourceTitle    string
	Content        string
	MetadataJSON   string
	Confidence     float64
	ExpiresAt      *time.Time
}

func NewMemoryService(
	memoryStore memory.Store,
	memoryRetriever memory.Retriever,
	conversationRepo *repository.ConversationRepository,
) *MemoryService {
	return &MemoryService{
		memoryStore:      memoryStore,
		memoryRetriever:  memoryRetriever,
		conversationRepo: conversationRepo,
	}
}

func (s *MemoryService) Create(
	ctx context.Context,
	userID int64,
	input CreateMemoryInput,
) (*entity.MemoryItem, error) {
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return nil, ErrInvalidMemoryInput
	}

	metadataJSON := strings.TrimSpace(input.MetadataJSON)
	if metadataJSON == "" {
		metadataJSON = "{}"
	}
	if !json.Valid([]byte(metadataJSON)) {
		return nil, ErrInvalidMemoryInput
	}

	scope := strings.TrimSpace(input.Scope)
	if scope == "" {
		scope = memory.MemoryScopeUser
	}

	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = memory.MemoryCategoryFact
	}

	sourceType := strings.TrimSpace(input.SourceType)
	if sourceType == "" {
		sourceType = memory.MemorySourceTypeNone
	}

	origin := strings.TrimSpace(input.Origin)
	if !isValidCreateMemoryScope(scope) {
		return nil, ErrInvalidMemoryInput
	}
	if !isValidMemoryCategory(category) {
		return nil, ErrInvalidMemoryInput
	}
	if !isValidMemoryOrigin(origin) {
		return nil, ErrInvalidMemoryInput
	}
	if !isValidMemorySourceType(sourceType) {
		return nil, ErrInvalidMemoryInput
	}

	confidence := input.Confidence
	if confidence == 0 {
		confidence = 1
	}
	if confidence < 0 || confidence > 1 {
		return nil, ErrInvalidMemoryInput
	}

	item := &entity.MemoryItem{
		Scope:        scope,
		Category:     category,
		Origin:       origin,
		SourceType:   sourceType,
		SourceRef:    nullString(input.SourceRef),
		SourceTitle:  nullString(input.SourceTitle),
		Content:      content,
		MetadataJSON: metadataJSON,
		Confidence:   confidence,
		ExpiresAt:    nullTime(input.ExpiresAt),
		Status:       memory.MemoryStatusActive,
	}

	switch scope {
	case memory.MemoryScopeUser:
		item.OwnerUserID = sql.NullInt64{Int64: userID, Valid: true}

	case memory.MemoryScopeConversation:
		conversationID := strings.TrimSpace(input.ConversationID)
		if conversationID == "" {
			return nil, ErrInvalidMemoryInput
		}
		if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
			return nil, err
		}
		item.OwnerUserID = sql.NullInt64{Int64: userID, Valid: true}
		item.ConversationID = sql.NullString{String: conversationID, Valid: true}

	default:
		return nil, ErrInvalidMemoryInput
	}

	if err := s.memoryStore.Create(ctx, item); err != nil {
		return nil, err
	}

	return s.memoryStore.GetByID(ctx, userID, item.ID)
}

func (s *MemoryService) ListOwned(
	ctx context.Context,
	userID int64,
	statuses []string,
	categories []string,
	scopes []string,
	limit int,
	offset int,
) ([]entity.MemoryItem, error) {
	statuses, err := normalizeMemoryStatuses(statuses)
	if err != nil {
		return nil, err
	}
	categories, err = normalizeMemoryCategories(categories)
	if err != nil {
		return nil, err
	}
	scopes, err = normalizeOwnedMemoryScopes(scopes)
	if err != nil {
		return nil, err
	}

	return s.memoryStore.List(ctx, memory.ListFilter{
		UserID:     userID,
		Statuses:   statuses,
		Categories: categories,
		Scopes:     scopes,
		Limit:      limit,
		Offset:     offset,
	})
}

func (s *MemoryService) Enable(ctx context.Context, userID int64, id int64) error {
	return s.updateStatus(ctx, userID, id, memory.MemoryStatusActive)
}

func (s *MemoryService) Disable(ctx context.Context, userID int64, id int64) error {
	return s.updateStatus(ctx, userID, id, memory.MemoryStatusDisabled)
}

func (s *MemoryService) Delete(ctx context.Context, userID int64, id int64) error {
	deleted, err := s.memoryStore.SoftDelete(ctx, userID, id)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrMemoryItemNotFound
	}
	return nil
}

func (s *MemoryService) SearchActive(
	ctx context.Context,
	userID int64,
	conversationID string,
	query string,
	keywords []string,
	limit int,
	categories []string,
	scopes []string,
) ([]memory.SearchHit, error) {
	categories, err := normalizeMemoryCategories(categories)
	if err != nil {
		return nil, err
	}
	scopes, err = normalizeSearchMemoryScopes(scopes)
	if err != nil {
		return nil, err
	}

	if conversationID != "" {
		if err := s.ensureConversationOwned(ctx, userID, conversationID); err != nil {
			return nil, err
		}
	}

	return s.memoryRetriever.Search(ctx, memory.SearchInput{
		UserID:         userID,
		ConversationID: conversationID,
		Query:          query,
		Keywords:       keywords,
		Categories:     categories,
		Scopes:         scopes,
		Limit:          limit,
	})
}

func (s *MemoryService) ListPromptMemories(
	ctx context.Context,
	userID int64,
	limit int,
) ([]entity.MemoryItem, error) {
	return s.memoryStore.ListPrompt(ctx, memory.PromptFilter{
		UserID: userID,
		Limit:  limit,
	})
}

func (s *MemoryService) updateStatus(ctx context.Context, userID int64, id int64, status string) error {
	item, err := s.memoryStore.GetByID(ctx, userID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMemoryItemNotFound
		}
		return err
	}

	if item.Scope == memory.MemoryScopeGlobal {
		return ErrMemoryItemNotFound
	}

	if item.Status == memory.MemoryStatusDeleted {
		return ErrMemoryItemNotFound
	}

	updated, err := s.memoryStore.UpdateStatus(ctx, userID, id, status)
	if err != nil {
		return err
	}
	if !updated {
		return ErrMemoryItemNotFound
	}

	return nil
}

func (s *MemoryService) ensureConversationOwned(ctx context.Context, userID int64, conversationID string) error {
	_, err := s.conversationRepo.GetByIDAndUserID(ctx, conversationID, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConversationNotFound
		}
		return err
	}
	return nil
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}

func nullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func isValidCreateMemoryScope(value string) bool {
	return isAllowed(value, allowedPrivateMemoryScopes)
}

func isValidMemoryCategory(value string) bool {
	return isAllowed(value, allowedMemoryCategories)
}

func isValidMemoryOrigin(value string) bool {
	return isAllowed(value, allowedMemoryOrigins)
}

func isValidMemorySourceType(value string) bool {
	return isAllowed(value, allowedMemorySourceTypes)
}

func normalizeMemoryStatuses(values []string) ([]string, error) {
	return normalizeAllowedValues(values, allowedMemoryStatuses)
}

func normalizeMemoryCategories(values []string) ([]string, error) {
	return normalizeAllowedValues(values, allowedMemoryCategories)
}

func normalizeOwnedMemoryScopes(values []string) ([]string, error) {
	return normalizeAllowedValues(values, allowedPrivateMemoryScopes)
}

func normalizeSearchMemoryScopes(values []string) ([]string, error) {
	return normalizeAllowedValues(values, allowedSearchMemoryScopes)
}

func isAllowed(value string, allowed []string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func normalizeAllowedValues(values []string, allowed []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}

	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if !isAllowed(value, allowed) {
			return nil, ErrInvalidMemoryInput
		}
		if _, ok := seen[value]; ok {
			continue
		}

		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result, nil
}
