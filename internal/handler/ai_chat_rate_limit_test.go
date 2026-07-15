package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIChatClientKeyTrustsForwardedHeadersOnlyBehindTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil)
	req.RemoteAddr = "203.0.113.10:4567"
	req.Header.Set("X-Forwarded-For", "198.51.100.20, 192.0.2.30")
	req.Header.Set("X-Real-IP", "198.51.100.40")

	if got := aiChatClientKey(req, false); got != "203.0.113.10" {
		t.Fatalf("untrusted proxy key = %q, want direct peer", got)
	}
	if got := aiChatClientKey(req, true); got != "192.0.2.30" {
		t.Fatalf("trusted proxy key = %q, want right-most forwarded peer", got)
	}
}
