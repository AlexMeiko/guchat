package embed

import "context"

type EmbedInput struct {
	Texts []string
}

type EmbedResult struct {
	Vectors [][]float32
	Model   string
	Dim     int
}

type Embedder interface {
	EmbedTexts(ctx context.Context, input EmbedInput) (*EmbedResult, error)
}
