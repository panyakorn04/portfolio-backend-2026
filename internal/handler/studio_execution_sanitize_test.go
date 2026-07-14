package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeStudioExecutionItemsRedactsAndBoundsOutput(t *testing.T) {
	t.Parallel()

	items := []map[string]any{{"json": map[string]any{
		"safe":    "visible",
		"plain":   "Authorization: Bearer very-secret-token and sk-live-example",
		"neutral": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0.signature12345",
		"nested":  map[string]any{"access_token": "secret-value", "password": "hidden"},
	}}}
	sanitized := sanitizeStudioExecutionItems(items)
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "secret-value") || strings.Contains(text, "hidden") || strings.Contains(text, "very-secret-token") || strings.Contains(text, "sk-live-example") || strings.Contains(text, "eyJhbGci") || !strings.Contains(text, "[REDACTED]") || !strings.Contains(text, "visible") {
		t.Fatalf("unexpected sanitized output: %s", text)
	}
	if items[0]["json"].(map[string]any)["nested"].(map[string]any)["access_token"] != "secret-value" {
		t.Fatal("sanitizer mutated caller-owned output")
	}

	large := []map[string]any{{"json": map[string]any{"text": strings.Repeat("x", maxStudioPersistedItemsBytes)}}}
	truncated := sanitizeStudioExecutionItems(large)
	if truncated[0]["json"].(map[string]any)["truncated"] != true {
		t.Fatalf("oversized output was not replaced with truncation marker: %#v", truncated)
	}
}
