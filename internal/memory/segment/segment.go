package segment

import (
	"context"
	"errors"
	"strings"
)

const (
	ModalityText = "text"
)

var ErrInvalidSplitInput = errors.New("invalid split input")

type SplitInput struct {
	Modality    string
	Content     string
	SourceTitle string
}

type Segment struct {
	Index           int
	Modality        string
	Content         string
	SplitterVersion string
	StartOffset     int
	EndOffset       int
}

type Splitter interface {
	Version() string
	Split(ctx context.Context, input SplitInput) ([]Segment, error)
}

func normalizeSplitInput(input SplitInput) (SplitInput, error) {
	input.Modality = strings.TrimSpace(input.Modality)
	if input.Modality == "" {
		input.Modality = ModalityText
	}
	if input.Modality != ModalityText {
		return SplitInput{}, ErrInvalidSplitInput
	}

	if strings.TrimSpace(input.Content) == "" {
		return SplitInput{}, ErrInvalidSplitInput
	}

	return input, nil
}
