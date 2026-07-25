package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPortfolioChatWriteRateLimitsCreateAndHandoffSeparately(t *testing.T) {
	for i := 0; i < chatCreateRateLimit; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/sessions", nil)
		request.RemoteAddr = "203.0.113.91:4321"
		if !enforcePortfolioChatWriteRateLimit(recorder, request, nil, "create") {
			t.Fatalf("create request %d was unexpectedly blocked", i+1)
		}
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/sessions", nil)
	request.RemoteAddr = "203.0.113.91:4321"
	if enforcePortfolioChatWriteRateLimit(recorder, request, nil, "create") {
		t.Fatal("create request above the limit was allowed")
	}
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" {
		t.Fatalf("status = %d, retry-after = %q", recorder.Code, recorder.Header().Get("Retry-After"))
	}

	handoffRecorder := httptest.NewRecorder()
	handoffRequest := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/sessions/session-1/request-human", nil)
	handoffRequest.RemoteAddr = "203.0.113.91:4321"
	if !enforcePortfolioChatWriteRateLimit(handoffRecorder, handoffRequest, nil, "handoff") {
		t.Fatal("handoff should use a separate rate-limit bucket")
	}
}
