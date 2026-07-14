package handler

import (
	"encoding/json"
	"fmt"
	"regexp"
)

const maxStudioPersistedItemsBytes = 256 << 10

var studioPersistedSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`),
	regexp.MustCompile(`\b(?:sk-|ghp_|github_pat_)[A-Za-z0-9_.-]{8,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)(authorization|api[_ -]?key|api[_ -]?token|access[_ -]?token|refresh[_ -]?token|client[_ -]?secret|token|secret|password|passwd|cookie|signature)\s*[:=]\s*[^\s,;]+`),
}

func redactStudioPersistedString(value string) string {
	for _, pattern := range studioPersistedSecretPatterns {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	return value
}

func sanitizeStudioExecutionItems(items []map[string]any) []map[string]any {
	if items == nil {
		return []map[string]any{}
	}
	encoded, err := json.Marshal(items)
	if err != nil {
		return []map[string]any{{"json": map[string]any{"error": "Output could not be serialized."}}}
	}
	var cloned []map[string]any
	if json.Unmarshal(encoded, &cloned) != nil {
		return []map[string]any{{"json": map[string]any{"error": "Output could not be serialized."}}}
	}
	for _, item := range cloned {
		redactStudioExecutionValue(item)
	}
	encoded, err = json.Marshal(cloned)
	if err != nil || len(encoded) > maxStudioPersistedItemsBytes {
		return []map[string]any{{"json": map[string]any{
			"truncated": true,
			"message":   fmt.Sprintf("Execution data exceeded the %d byte persistence limit.", maxStudioPersistedItemsBytes),
		}}}
	}
	return cloned
}

func redactStudioExecutionValue(value any) any {
	switch typed := value.(type) {
	case string:
		return redactStudioPersistedString(typed)
	case map[string]any:
		for key, child := range typed {
			if isStudioSecretFieldName(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			typed[key] = redactStudioExecutionValue(child)
		}
	case []any:
		for index, child := range typed {
			typed[index] = redactStudioExecutionValue(child)
		}
	}
	return value
}
