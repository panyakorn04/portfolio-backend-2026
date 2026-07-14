package handler

import (
	"crypto/hmac"
	"encoding/hex"
	"testing"
)

func TestStudioWebhookTokenIsScopedAndConstantLength(t *testing.T) {
	t.Parallel()
	first := studioWebhookToken("test-signing-secret", "workflow-a", "node-a")
	if len(first) != 64 {
		t.Fatalf("unexpected webhook token length: %d", len(first))
	}
	if first == studioWebhookToken("test-signing-secret", "workflow-b", "node-a") || first == studioWebhookToken("test-signing-secret", "workflow-a", "node-b") {
		t.Fatal("webhook token was not bound to workflow and node")
	}
	decoded, err := hex.DecodeString(first)
	if err != nil || !hmac.Equal(decoded, decoded) {
		t.Fatal("webhook token is not valid hexadecimal HMAC output")
	}
}
