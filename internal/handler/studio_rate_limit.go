package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const (
	loginRateLimit       = 5
	loginRateWindow      = 15 * time.Minute
	studioMutationLimit  = 30
	studioMutationWindow = time.Minute
	contactRateLimit     = 5
	contactRateWindow    = 10 * time.Minute
)

type rateWindow struct {
	count int
	reset time.Time
}

type memoryRateLimiter struct {
	mu        sync.Mutex
	windows   map[string]rateWindow
	lastSweep time.Time
}

func newMemoryRateLimiter() *memoryRateLimiter {
	return &memoryRateLimiter{windows: make(map[string]rateWindow)}
}

func (l *memoryRateLimiter) Allow(key string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= time.Minute {
		for existingKey, existing := range l.windows {
			if !now.Before(existing.reset) {
				delete(l.windows, existingKey)
			}
		}
		l.lastSweep = now
	}
	v := l.windows[key]
	if v.reset.IsZero() || !now.Before(v.reset) {
		l.windows[key] = rateWindow{count: 1, reset: now.Add(window)}
		return true, 0
	}
	if v.count >= limit {
		return false, v.reset.Sub(now)
	}
	v.count++
	l.windows[key] = v
	return true, 0
}

var studioLimiter = newMemoryRateLimiter()

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if raw := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); raw != "" {
			parts := strings.Split(raw, ",")
			// Caddy appends the direct client address. Use the right-most valid
			// hop so a caller-supplied left-most value cannot bypass limits.
			for i := len(parts) - 1; i >= 0; i-- {
				candidate := strings.TrimSpace(parts[i])
				if net.ParseIP(candidate) != nil {
					return candidate
				}
			}
		}
		if candidate := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func limited(s *svc.ServiceContext, key string, limit int, window time.Duration, now time.Time) (bool, time.Duration) {
	if s != nil && s.ArticleCache != nil && s.ArticleCache.Enabled() {
		ok, retry, err := s.ArticleCache.Allow(ratelimitKey(key), limit, window)
		if err == nil {
			return !ok, retry
		}
	}
	ok, retry := studioLimiter.Allow(key, limit, window, now)
	return !ok, retry
}

func ratelimitKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "portfolio:rate:" + hex.EncodeToString(sum[:])
}

func writeRateLimited(w http.ResponseWriter, retry time.Duration) {
	seconds := int(math.Ceil(retry.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", fmtInt(seconds))
	response.Error(w, http.StatusTooManyRequests, "Too many requests. Try again later.")
}

func fmtInt(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}

func enforceLoginRateLimit(w http.ResponseWriter, r *http.Request, s *svc.ServiceContext) bool {
	ip := clientIP(r, s != nil && s.Config.TrustProxy)
	blocked, retry := limited(s, "login:ip:"+ip, loginRateLimit, loginRateWindow, time.Now())
	if blocked {
		writeRateLimited(w, retry)
		return false
	}
	return true
}

func enforceContactRateLimit(w http.ResponseWriter, r *http.Request, s *svc.ServiceContext) bool {
	ip := clientIP(r, s != nil && s.Config.TrustProxy)
	if blocked, retry := limited(s, "contact:ip:"+ip, contactRateLimit, contactRateWindow, time.Now()); blocked {
		writeRateLimited(w, retry)
		return false
	}
	return true
}

func enforceStudioMutationRateLimit(w http.ResponseWriter, r *http.Request, s *svc.ServiceContext, access *auth.AccessContext) bool {
	ip := clientIP(r, s != nil && s.Config.TrustProxy)
	keys := []string{"studio:ip:" + ip}
	if access.Via == auth.ViaSession {
		raw := auth.GetCookieValue(r, auth.SessionCookieName)
		keys = append(keys, "studio:session:"+auth.HashSessionToken(raw))
	} else {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		keys = append(keys, "studio:bearer:"+ratelimitKey(token))
	}
	for _, key := range keys {
		if blocked, retry := limited(s, key, studioMutationLimit, studioMutationWindow, time.Now()); blocked {
			writeRateLimited(w, retry)
			return false
		}
	}
	return true
}
