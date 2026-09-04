package generator

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/AlexMeiko/guchat/internal/service"
)

var errOpenAIStreamDone = errors.New("stream done")

const maxSSELineBytes = 4 << 20

var toolCallTagRe = regexp.MustCompile(`<!--tool_call:([^>]+)-->`)

func forEachSSELine(r io.Reader, fn func(line string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxSSELineBytes)
	for scanner.Scan() {
		if err := fn(strings.TrimRight(scanner.Text(), "\r")); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("sse line exceeds %d bytes", maxSSELineBytes)
		}
		return err
	}
	return nil
}

func marshalOpenAIRequestBody(base any, extraBody string, reservedKeys ...string) ([]byte, error) {
	basePayload, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(extraBody) == "" {
		return basePayload, nil
	}

	var body map[string]any
	if err := json.Unmarshal(basePayload, &body); err != nil {
		return nil, err
	}

	var extra map[string]any
	if err := json.Unmarshal([]byte(extraBody), &extra); err != nil {
		return nil, err
	}

	reserved := make(map[string]struct{}, len(reservedKeys))
	for _, key := range reservedKeys {
		reserved[key] = struct{}{}
	}

	for key, value := range extra {
		if _, ok := reserved[key]; ok {
			return nil, fmt.Errorf("extra_body contains reserved key: %s", key)
		}
		body[key] = value
	}

	return json.Marshal(body)
}

func sliceByOffset(content string, start, end int) string {
	if start < 0 || end < start || start > len(content) {
		return ""
	}

	end = min(end, len(content))

	return strings.TrimSpace(content[start:end])
}

func groupToolExchangesByRound(exchanges []service.ToolExchange) map[int][]service.ToolExchange {
	result := map[int][]service.ToolExchange{}

	for _, exchange := range exchanges {
		result[exchange.Round] = append(result[exchange.Round], exchange)
	}

	return result
}
