package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var ErrDashScopeEmbedder = errors.New("dashscope embedder error")

type DashScopeEmbedder struct {
	url    string
	apiKey string
	model  string
	dim    int
	client *http.Client
}

type DashScopeEmbedderConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Dim     int
}

type dashScopeEmbeddingRequest struct {
	Model      string                       `json:"model"`
	Input      dashScopeEmbeddingInput      `json:"input"`
	Parameters dashScopeEmbeddingParameters `json:"parameters,omitempty"`
}

type dashScopeEmbeddingInput struct {
	Texts []string `json:"texts"`
}

type dashScopeEmbeddingParameters struct {
	Dimension int `json:"dimension,omitempty"`
}

type dashScopeEmbeddingResponse struct {
	Output struct {
		Embeddings []struct {
			TextIndex int       `json:"text_index"`
			Embedding []float64 `json:"embedding"`
		} `json:"embeddings"`
	} `json:"output"`
}

func NewDashScopeEmbedder(client *http.Client, cfg DashScopeEmbedderConfig) (*DashScopeEmbedder, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	model := strings.TrimSpace(cfg.Model)

	if baseURL == "" || apiKey == "" || model == "" || cfg.Dim <= 0 {
		return nil, ErrInvalidEmbedderConfig
	}

	if client == nil {
		client = http.DefaultClient
	}

	return &DashScopeEmbedder{
		url:    baseURL,
		apiKey: apiKey,
		model:  model,
		dim:    cfg.Dim,
		client: client,
	}, nil
}

func (e *DashScopeEmbedder) EmbedTexts(ctx context.Context, input EmbedInput) (*EmbedResult, error) {
	if err := validateEmbedTexts(input.Texts); err != nil {
		return nil, err
	}

	reqBody, err := json.Marshal(dashScopeEmbeddingRequest{
		Model:      e.model,
		Input:      dashScopeEmbeddingInput{Texts: input.Texts},
		Parameters: dashScopeEmbeddingParameters{Dimension: e.dim},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("%w: status %d: %s", ErrDashScopeEmbedder, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded dashScopeEmbeddingResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}

	vectors, err := buildDashScopeVectors(decoded.Output.Embeddings, len(input.Texts))
	if err != nil {
		return nil, err
	}
	if len(vectors[0]) != e.dim {
		return nil, fmt.Errorf("%w: embedding dimension mismatch: expected %d, got %d", ErrDashScopeEmbedder, e.dim, len(vectors[0]))
	}

	return &EmbedResult{
		Vectors: vectors,
		Model:   e.model,
		Dim:     len(vectors[0]),
	}, nil
}

func buildDashScopeVectors(embeddings []struct {
	TextIndex int       `json:"text_index"`
	Embedding []float64 `json:"embedding"`
}, expected int) ([][]float32, error) {
	if len(embeddings) != expected {
		return nil, fmt.Errorf("%w: embedding count mismatch", ErrDashScopeEmbedder)
	}

	vectors := make([][]float32, expected)
	for _, item := range embeddings {
		if item.TextIndex < 0 || item.TextIndex >= expected {
			return nil, fmt.Errorf("%w: text_index out of range", ErrDashScopeEmbedder)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("%w: empty embedding", ErrDashScopeEmbedder)
		}

		vector := make([]float32, len(item.Embedding))
		for i, value := range item.Embedding {
			vector[i] = float32(value)
		}
		vectors[item.TextIndex] = vector
	}

	dim := 0
	for _, vector := range vectors {
		if len(vector) == 0 {
			return nil, fmt.Errorf("%w: missing embedding", ErrDashScopeEmbedder)
		}
		if dim == 0 {
			dim = len(vector)
			continue
		}
		if len(vector) != dim {
			return nil, fmt.Errorf("%w: embedding dimension mismatch", ErrDashScopeEmbedder)
		}
	}

	return vectors, nil
}
