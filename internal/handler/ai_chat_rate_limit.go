package handler

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type aiChatRateLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	requests map[string][]time.Time
}

func newAIChatRateLimiter(limit int, window time.Duration) *aiChatRateLimiter {
	return &aiChatRateLimiter{
		limit:    limit,
		window:   window,
		requests: make(map[string][]time.Time),
	}
}

func (l *aiChatRateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.window)
	existing := l.requests[key]
	kept := existing[:0]
	for _, ts := range existing {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}

	if len(kept) >= l.limit {
		l.requests[key] = kept
		return false
	}

	kept = append(kept, now)
	l.requests[key] = kept
	return true
}

func aiChatClientKey(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		candidate := strings.TrimSpace(parts[len(parts)-1])
		if candidate != "" {
			return candidate
		}
	}

	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}

	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
