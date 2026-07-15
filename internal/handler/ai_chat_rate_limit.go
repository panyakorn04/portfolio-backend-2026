package handler

import (
	"net/http"
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

func aiChatClientKey(r *http.Request, trustProxy bool) string {
	return clientIP(r, trustProxy)
}
