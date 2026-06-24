package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidQdrantConfig = errors.New("invalid qdrant config")
var ErrInvalidVectorInput = errors.New("invalid vector input")
var ErrQdrant = errors.New("qdrant error")

type QdrantError struct {
	StatusCode int
	Body       string
}

func (e *QdrantError) Error() string {
	return fmt.Sprintf("%v: status %d: %s", ErrQdrant, e.StatusCode, e.Body)
}

func (e *QdrantError) Unwrap() error {
	return ErrQdrant
}

type QdrantConfig struct {
	BaseURL    string
	APIKey     string
	Collection string
	VectorSize int
	Distance   string
}

type QdrantIndex struct {
	baseURL    string
	apiKey     string
	collection string
	vectorSize int
	distance   string
	client     *http.Client
}

type qdrantUpsertRequest struct {
	Points []qdrantPoint `json:"points"`
}

type qdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

type qdrantDeleteRequest struct {
	Filter qdrantFilter `json:"filter"`
}

type qdrantFilter struct {
	Must []qdrantCondition `json:"must"`
}

type qdrantCondition struct {
	Key   string      `json:"key"`
	Match qdrantMatch `json:"match"`
}

type qdrantMatch struct {
	Value any `json:"value"`
}

type qdrantSearchRequest struct {
	Vector      []float32     `json:"vector"`
	Limit       int           `json:"limit"`
	WithPayload bool          `json:"with_payload"`
	Filter      *qdrantFilter `json:"filter,omitempty"`
}

type qdrantSearchResponse struct {
	Result []qdrantSearchResult `json:"result"`
}

type qdrantSearchResult struct {
	Payload map[string]any `json:"payload"`
}

type qdrantCreateCollectionRequest struct {
	Vectors qdrantVectorParams `json:"vectors"`
}

type qdrantVectorParams struct {
	Size     int    `json:"size"`
	Distance string `json:"distance"`
}

type qdrantCreatePayloadIndexRequest struct {
	FieldName   string `json:"field_name"`
	FieldSchema string `json:"field_schema"`
}

func NewQdrantIndex(client *http.Client, cfg QdrantConfig) (*QdrantIndex, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	collection := strings.TrimSpace(cfg.Collection)

	if baseURL == "" || collection == "" {
		return nil, ErrInvalidQdrantConfig
	}
	if client == nil {
		client = http.DefaultClient
	}

	distance := strings.TrimSpace(cfg.Distance)
	if distance == "" {
		distance = "Cosine"
	}

	return &QdrantIndex{
		baseURL:    baseURL,
		apiKey:     apiKey,
		collection: collection,
		vectorSize: cfg.VectorSize,
		distance:   distance,
		client:     client,
	}, nil
}

func (q *QdrantIndex) Upsert(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}

	req := qdrantUpsertRequest{
		Points: make([]qdrantPoint, 0, len(points)),
	}

	for _, point := range points {
		if len(point.Vector) == 0 {
			return ErrInvalidVectorInput
		}

		req.Points = append(req.Points, qdrantPoint{
			ID:      uuid.NewString(),
			Vector:  point.Vector,
			Payload: point.Payload,
		})
	}

	path := fmt.Sprintf("/collections/%s/points?wait=true", q.collection)
	return q.doJSON(ctx, http.MethodPut, path, req, nil)
}

func (q *QdrantIndex) Search(ctx context.Context, input SearchInput) ([]SearchHit, error) {
	if len(input.Vector) == 0 {
		return nil, ErrInvalidVectorInput
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 5
	}

	req := qdrantSearchRequest{
		Vector:      input.Vector,
		Limit:       limit,
		WithPayload: true,
		Filter:      buildQdrantFilter(input.Filter),
	}

	var resp qdrantSearchResponse
	path := fmt.Sprintf("/collections/%s/points/search", q.collection)
	if err := q.doJSON(ctx, http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}

	hits := make([]SearchHit, 0, len(resp.Result))
	for _, item := range resp.Result {
		hits = append(hits, SearchHit{
			Payload: item.Payload,
		})
	}

	return hits, nil
}

func (q *QdrantIndex) DeleteByMemoryItemID(ctx context.Context, memoryItemID int64) error {
	if memoryItemID <= 0 {
		return ErrInvalidVectorInput
	}

	req := qdrantDeleteRequest{
		Filter: qdrantFilter{
			Must: []qdrantCondition{
				{
					Key: "memory_item_id",
					Match: qdrantMatch{
						Value: memoryItemID,
					},
				},
			},
		},
	}

	path := fmt.Sprintf("/collections/%s/points/delete?wait=true", q.collection)
	return q.doJSON(ctx, http.MethodPost, path, req, nil)
}

func (q *QdrantIndex) EnsureCollection(ctx context.Context) error {
	if q.vectorSize <= 0 {
		return ErrInvalidQdrantConfig
	}

	req := qdrantCreateCollectionRequest{
		Vectors: qdrantVectorParams{
			Size:     q.vectorSize,
			Distance: q.distance,
		},
	}

	path := fmt.Sprintf("/collections/%s", q.collection)
	if err := q.doJSON(ctx, http.MethodPut, path, req, nil); err != nil {
		if isQdrantAlreadyExists(err) {
			return nil
		}
		return err
	}

	return nil
}

func (q *QdrantIndex) EnsureIndexes(ctx context.Context) error {
	indexes := []qdrantCreatePayloadIndexRequest{
		{FieldName: "memory_item_id", FieldSchema: "integer"},
		{FieldName: "owner_user_id", FieldSchema: "integer"},
		{FieldName: "scope", FieldSchema: "keyword"},
		{FieldName: "conversation_id", FieldSchema: "keyword"},
		{FieldName: "status", FieldSchema: "keyword"},
		{FieldName: "category", FieldSchema: "keyword"},
	}

	path := fmt.Sprintf("/collections/%s/index", q.collection)
	for _, item := range indexes {
		if err := q.doJSON(ctx, http.MethodPut, path, item, nil); err != nil {
			if isQdrantAlreadyExists(err) {
				continue
			}
			return err
		}
	}

	return nil
}

func (q *QdrantIndex) doJSON(
	ctx context.Context,
	method string,
	path string,
	reqBody any,
	respBody any,
) error {
	var body io.Reader
	if reqBody != nil {
		encoded, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, body)
	if err != nil {
		return err
	}

	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}

	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return &QdrantError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(body)),
		}
	}

	if respBody == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(respBody)
}

func isQdrantAlreadyExists(err error) bool {
	var qdrantErr *QdrantError
	if !errors.As(err, &qdrantErr) {
		return false
	}

	return qdrantErr.StatusCode == http.StatusConflict &&
		strings.Contains(strings.ToLower(qdrantErr.Body), "already exists")
}

func buildQdrantFilter(values map[string]any) *qdrantFilter {
	if len(values) == 0 {
		return nil
	}

	filter := &qdrantFilter{
		Must: make([]qdrantCondition, 0, len(values)),
	}

	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || value == nil {
			continue
		}

		filter.Must = append(filter.Must, qdrantCondition{
			Key: key,
			Match: qdrantMatch{
				Value: value,
			},
		})
	}

	if len(filter.Must) == 0 {
		return nil
	}

	return filter
}
