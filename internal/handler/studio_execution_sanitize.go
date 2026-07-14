package handler

import (
	"encoding/json"
	"fmt"
)

const maxStudioPersistedItemsBytes = 256 << 10

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

func redactStudioExecutionValue(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isStudioSecretFieldName(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			redactStudioExecutionValue(child)
		}
	case []any:
		for _, child := range typed {
			redactStudioExecutionValue(child)
		}
	}
}
