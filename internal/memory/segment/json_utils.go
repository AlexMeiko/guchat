package segment

import (
	"errors"
	"strings"
)

var ErrInvalidJSONFieldPath = errors.New("invalid json field path")

func getJSONFieldPath(root any, path string) (any, error) {
	parts, err := parseJSONFieldPath(path)
	if err != nil {
		return nil, err
	}

	current := root
	for _, part := range parts {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, ErrInvalidJSONFieldPath
		}

		next, ok := object[part]
		if !ok {
			return nil, ErrInvalidJSONFieldPath
		}

		current = next
	}

	return current, nil
}

func parseJSONFieldPath(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, ErrInvalidJSONFieldPath
	}

	parts := strings.Split(path, ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return nil, ErrInvalidJSONFieldPath
		}
	}

	return parts, nil
}
