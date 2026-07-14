package handler

import (
	"crypto/hmac"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"portfolio-backend/internal/model"
)

func TestStudioWebhookTokenIsScopedVersionedAndConstantLength(t *testing.T) {
	t.Parallel()
	first := studioWebhookToken("test-signing-secret", "workflow-a", "node-a", "version-a")
	if len(first) != 64 {
		t.Fatalf("unexpected webhook token length: %d", len(first))
	}
	if first == studioWebhookToken("test-signing-secret", "workflow-b", "node-a", "version-a") ||
		first == studioWebhookToken("test-signing-secret", "workflow-a", "node-b", "version-a") ||
		first == studioWebhookToken("test-signing-secret", "workflow-a", "node-a", "version-b") {
		t.Fatal("webhook token was not bound to workflow, node, and revocation version")
	}
	if validStudioWebhookSigningKey("short", "different") || validStudioWebhookSigningKey(strings.Repeat("a", 32), strings.Repeat("a", 32)) || !validStudioWebhookSigningKey(strings.Repeat("a", 32), strings.Repeat("b", 32)) {
		t.Fatal("webhook signing-key isolation validation is incorrect")
	}
	decoded, err := hex.DecodeString(first)
	if err != nil || !hmac.Equal(decoded, decoded) {
		t.Fatal("webhook token is not valid hexadecimal HMAC output")
	}
}

func TestEnsureStudioWebhookTokenVersionsAndBodyRedaction(t *testing.T) {
	t.Parallel()
	definition := &model.StudioWorkflowDefinition{Nodes: []model.StudioWorkflowNode{{
		ID: "hook", Type: "webhook", Kind: "trigger", Config: map[string]any{"enabled": true},
	}}}
	if !ensureStudioWebhookTokenVersions(definition) {
		t.Fatal("failed to initialize webhook capability version")
	}
	first := studioWebhookTokenVersion(&definition.Nodes[0])
	if decoded, err := hex.DecodeString(first); err != nil || len(decoded) != 16 {
		t.Fatalf("invalid generated webhook version: %q", first)
	}
	if !ensureStudioWebhookTokenVersions(definition) || studioWebhookTokenVersion(&definition.Nodes[0]) != first {
		t.Fatal("valid webhook version was not preserved")
	}

	form, err := parseStudioWebhookBody("application/x-www-form-urlencoded", []byte("safe=value&password=hidden&token=secret"))
	formText := strings.ToLower(fmt.Sprint(form))
	if err != nil || strings.Contains(formText, "hidden") || strings.Contains(formText, "token=secret") {
		t.Fatalf("form webhook secrets leaked: %#v, %v", form, err)
	}
	raw, err := parseStudioWebhookBody("text/plain", []byte(`{"password":"hidden","token":"secret"}`))
	if err != nil || strings.Contains(raw.(string), "hidden") || strings.Contains(raw.(string), "secret") {
		t.Fatalf("raw webhook secrets leaked: %#v, %v", raw, err)
	}
}
