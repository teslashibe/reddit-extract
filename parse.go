package redditextract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseResponse decodes a model response into the requested output type.
func ParseResponse[T any](response string) (T, error) {
	var out T

	jsonPayload, err := ExtractJSON(response)
	if err != nil {
		return out, err
	}

	if err := json.Unmarshal([]byte(jsonPayload), &out); err != nil {
		return out, fmt.Errorf("unmarshal response: %w", err)
	}

	return out, nil
}

// ExtractJSON extracts the first valid JSON object/array from a model response.
func ExtractJSON(response string) (string, error) {
	clean := strings.TrimSpace(response)
	if clean == "" {
		return "", fmt.Errorf("empty response")
	}

	candidates := []string{clean}
	if stripped := stripCodeFence(clean); stripped != clean {
		candidates = append([]string{stripped}, candidates...)
	}

	for _, candidate := range candidates {
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
		if extracted := extractBracketed(candidate, '{', '}'); extracted != "" && json.Valid([]byte(extracted)) {
			return extracted, nil
		}
		if extracted := extractBracketed(candidate, '[', ']'); extracted != "" && json.Valid([]byte(extracted)) {
			return extracted, nil
		}
	}

	return "", fmt.Errorf("no valid JSON found in response")
}

func stripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	trimmed := strings.TrimPrefix(s, "```")
	if idx := strings.Index(trimmed, "\n"); idx >= 0 {
		trimmed = trimmed[idx+1:]
	}
	if end := strings.LastIndex(trimmed, "```"); end >= 0 {
		trimmed = trimmed[:end]
	}
	return strings.TrimSpace(trimmed)
}

func extractBracketed(s string, left, right rune) string {
	start := strings.IndexRune(s, left)
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, string(right))
	if end <= start {
		return ""
	}
	return s[start : end+1]
}
