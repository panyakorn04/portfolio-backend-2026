package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portfolio-backend/internal/svc"
)

const validContactJSON = `{"name":"Alice","email":"alice@example.com","subject":"Project inquiry","message":"This is a sufficiently long contact message.","locale":"en"}`

func TestContactHandlerRejectsOversizedBody(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(`{"message":"`+strings.Repeat("x", int(maxContactBodyBytes))+`"}`))
	request.RemoteAddr = "192.0.2.10:1234"
	recorder := httptest.NewRecorder()

	ContactHandler(&svc.ServiceContext{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
}

func TestContactHandlerRateLimitsByClientIP(t *testing.T) {
	service := &svc.ServiceContext{}
	for attempt := 1; attempt <= contactRateLimit+1; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(validContactJSON))
		request.RemoteAddr = "192.0.2.20:1234"
		recorder := httptest.NewRecorder()

		ContactHandler(service).ServeHTTP(recorder, request)
		if attempt <= contactRateLimit && recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("attempt %d status = %d, want %d", attempt, recorder.Code, http.StatusServiceUnavailable)
		}
		if attempt == contactRateLimit+1 && recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("rate-limited status = %d, want %d; body=%s", recorder.Code, http.StatusTooManyRequests, recorder.Body.String())
		}
	}
}

func TestMemoryRateLimiterSweepsExpiredKeys(t *testing.T) {
	limiter := newMemoryRateLimiter()
	start := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.UTC)
	limiter.Allow("expired", 1, time.Second, start)
	limiter.Allow("current", 1, time.Minute, start.Add(2*time.Minute))
	if _, exists := limiter.windows["expired"]; exists {
		t.Fatal("expired rate-limit key was not swept")
	}
}
