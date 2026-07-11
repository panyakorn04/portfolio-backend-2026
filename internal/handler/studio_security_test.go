package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPDoesNotTrustForwardedHeadersByDefault(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/admin/session", nil)
	r.RemoteAddr = "192.0.2.10:4321"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.99")
	if got := clientIP(r, false); got != "192.0.2.10" {
		t.Fatalf("clientIP=%q", got)
	}
	if got := clientIP(r, true); got != "203.0.113.99" {
		t.Fatalf("trusted proxy clientIP=%q", got)
	}
}

func TestMemoryRateLimiterReturnsRetryAfter(t *testing.T) {
	l := newMemoryRateLimiter()
	now := time.Unix(100, 0)
	if ok, _ := l.Allow("login:ip", 1, time.Minute, now); !ok {
		t.Fatal("first request should pass")
	}
	if ok, retry := l.Allow("login:ip", 1, time.Minute, now.Add(time.Second)); ok || retry != 59*time.Second {
		t.Fatalf("ok=%v retry=%v", ok, retry)
	}
}

func TestRateLimitResponseIncludesRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	writeRateLimited(rec, 3*time.Second)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "3" {
		t.Fatalf("status=%d retry=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}
