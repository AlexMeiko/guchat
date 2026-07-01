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

var ErrOpenAIEmbedder = errors.New("openai embedder error")
var ErrInvalidEmbedderConfig = errors.New("invalid embedder config")
var ErrInvalidEmbedInput = errors.New("invalid embed input")

type OpenAIEmbedder struct {
	url    string
	apiKey string
	model  string
	client *http.Client
}

type OpenAIEmbedderConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

type openAIEmbeddingRequest struct {
	Model          string   `json:"model"`
	Input          []string `json:"input"`
	EncodingFormat string   `json:"encoding_format"`
}

type openAIEmbeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func NewOpenAIEmbedder(client *http.Client, cfg OpenAIEmbedderConfig) (*OpenAIEmbedder, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	model := strings.TrimSpace(cfg.Model)

	if baseURL == "" || model == "" {
		return nil, ErrInvalidEmbedderConfig
	}

	if client == nil {
		client = http.DefaultClient
	}

	return &OpenAIEmbedder{
		url:    baseURL,
		apiKey: apiKey,
		model:  model,
		client: client,
	}, nil
}

func (e *OpenAIEmbedder) EmbedTexts(ctx context.Context, input EmbedInput) (*EmbedResult, error) {
	if err := validateEmbedTexts(input.Texts); err != nil {
		return nil, err
	}

	reqBody, err := json.Marshal(openAIEmbeddingRequest{
		Model:          e.model,
		Input:          input.Texts,
		EncodingFormat: "float",
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.apiKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("%w: status %d: %s", ErrOpenAIEmbedder, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded openAIEmbeddingResponse
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}

	vectors, err := buildEmbeddingVectors(decoded.Data, len(input.Texts))
	if err != nil {
		return nil, err
	}

	resultModel := strings.TrimSpace(decoded.Model)
	if resultModel == "" {
		resultModel = e.model
	}

	return &EmbedResult{
		Vectors: vectors,
		Model:   resultModel,
		Dim:     len(vectors[0]),
	}, nil
}

func validateEmbedTexts(values []string) error {
	if len(values) == 0 {
		return ErrInvalidEmbedInput
	}

	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return ErrInvalidEmbedInput
		}
	}

	return nil
}

func buildEmbeddingVectors(data []struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}, expected int) ([][]float32, error) {
	if len(data) != expected {
		return nil, fmt.Errorf("%w: embedding count mismatch", ErrOpenAIEmbedder)
	}

	vectors := make([][]float32, expected)
	for _, item := range data {
		if item.Index < 0 || item.Index >= expected {
			return nil, fmt.Errorf("%w: embedding index out of range", ErrOpenAIEmbedder)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("%w: empty embedding", ErrOpenAIEmbedder)
		}

		vector := make([]float32, len(item.Embedding))
		for i, value := range item.Embedding {
			vector[i] = float32(value)
		}

		vectors[item.Index] = vector
	}

	dim := 0
	for _, vector := range vectors {
		if len(vector) == 0 {
			return nil, fmt.Errorf("%w: missing embedding", ErrOpenAIEmbedder)
		}

		if dim == 0 {
			dim = len(vector)
			continue
		}
		if len(vector) != dim {
			return nil, fmt.Errorf("%w: embedding dimension mismatch", ErrOpenAIEmbedder)
		}
	}

	return vectors, nil
}
