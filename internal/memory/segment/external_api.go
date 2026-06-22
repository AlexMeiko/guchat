package segment

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

const ExternalAPISplitterVersion = "external_api_v1"

var ErrExternalAPISplitter = errors.New("external api splitter error")

type ExternalAPISplitter struct {
	URL          string
	Headers      map[string]string
	SegmentsPath string
	Client       *http.Client
}

func (s *ExternalAPISplitter) Version() string {
	return ExternalAPISplitterVersion
}

func (s *ExternalAPISplitter) Split(ctx context.Context, input SplitInput) ([]Segment, error) {
	input, err := normalizeSplitInput(input)
	if err != nil {
		return nil, err
	}

	reqBody, err := json.Marshal(map[string]any{
		"text": input.Content,
	})
	if err != nil {
		return nil, err
	}

	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(s.URL), bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range s.Headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%w: status %d", ErrExternalAPISplitter, resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decoded any
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, err
	}

	raw, err := getJSONFieldPath(decoded, s.SegmentsPath)
	if err != nil {
		return nil, err
	}

	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: segments is not array", ErrExternalAPISplitter)
	}

	return s.parseSegments(items)
}

func (s *ExternalAPISplitter) parseSegments(items []any) ([]Segment, error) {
	segments := make([]Segment, 0, len(items))
	segmentType := ""

	for _, item := range items {
		text, itemType, err := parseSegmentText(item)
		if err != nil {
			return nil, err
		}
		if segmentType == "" {
			segmentType = itemType
		}
		if segmentType != itemType {
			return nil, fmt.Errorf("%w: mixed segment item types", ErrExternalAPISplitter)
		}

		if strings.TrimSpace(text) == "" {
			continue
		}

		start, end, err := parseDefaultSegmentOffsets(item)
		if err != nil {
			return nil, err
		}

		segments = append(segments, Segment{
			Index:           len(segments),
			Modality:        ModalityText,
			Content:         text,
			SplitterVersion: s.Version(),
			StartOffset:     start,
			EndOffset:       end,
		})
	}

	if len(segments) == 0 {
		return nil, ErrInvalidSplitInput
	}

	return segments, nil
}

func parseSegmentText(item any) (string, string, error) {
	if text, ok := item.(string); ok {
		return text, "string", nil
	}

	object, ok := item.(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("%w: segment is not string or object", ErrExternalAPISplitter)
	}

	raw, ok := object["text"]
	if !ok {
		return "", "", fmt.Errorf("%w: object segment missing text", ErrExternalAPISplitter)
	}
	text, ok := raw.(string)
	if !ok {
		return "", "", fmt.Errorf("%w: segment text is not string", ErrExternalAPISplitter)
	}

	return text, "object", nil
}

func parseDefaultSegmentOffsets(item any) (int, int, error) {
	object, ok := item.(map[string]any)
	if !ok {
		return 0, 0, nil
	}

	rawStart, hasStart := object["start"]
	rawEnd, hasEnd := object["end"]
	if !hasStart && !hasEnd {
		return 0, 0, nil
	}
	if !hasStart || !hasEnd {
		return 0, 0, fmt.Errorf("%w: segment start and end must appear together", ErrExternalAPISplitter)
	}

	start, ok := jsonNumberToInt(rawStart)
	if !ok {
		return 0, 0, fmt.Errorf("%w: segment start is not integer", ErrExternalAPISplitter)
	}
	end, ok := jsonNumberToInt(rawEnd)
	if !ok {
		return 0, 0, fmt.Errorf("%w: segment end is not integer", ErrExternalAPISplitter)
	}
	if end < start {
		return 0, 0, fmt.Errorf("%w: segment end before start", ErrExternalAPISplitter)
	}

	return start, end, nil
}

func jsonNumberToInt(value any) (int, bool) {
	number, ok := value.(float64)
	if !ok {
		return 0, false
	}
	result := int(number)
	if number < 0 || float64(result) != number {
		return 0, false
	}
	return result, true
}
