package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

const RequestIDHeader = "X-Request-ID"

type (
	Level   string
	LogFunc func(context.Context, Level, string, ...logx.LogField)

	requestIDContextKey struct{}

	statusResponseWriter struct {
		http.ResponseWriter
		status      int
		wroteHeader bool
	}

	flushingStatusResponseWriter struct {
		*statusResponseWriter
		flusher http.Flusher
	}

	routePattern struct {
		raw      string
		segments []string
	}
)

const (
	LevelInfo  Level = "info"
	LevelError Level = "error"
)

func RequestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	return requestID
}

func HTTPMiddleware(logFn LogFunc, routePatterns ...string) func(http.Handler) http.Handler {
	if logFn == nil {
		logFn = Log
	}
	patterns := compileRoutePatterns(routePatterns)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			requestID := r.Header.Get(RequestIDHeader)
			if !validRequestID(requestID) {
				requestID = newRequestID()
			}

			w.Header().Set(RequestIDHeader, requestID)
			statusWriter := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
			var responseWriter http.ResponseWriter = statusWriter
			if flusher, ok := w.(http.Flusher); ok {
				responseWriter = &flushingStatusResponseWriter{statusResponseWriter: statusWriter, flusher: flusher}
			}
			ctx := context.WithValue(r.Context(), requestIDContextKey{}, requestID)
			next.ServeHTTP(responseWriter, r.WithContext(ctx))

			level := LevelInfo
			if statusWriter.status >= http.StatusInternalServerError {
				level = LevelError
			}
			logFn(ctx, level, "http request completed",
				logx.Field("event", "http.request.completed"),
				logx.Field("request_id", requestID),
				logx.Field("method", r.Method),
				logx.Field("route", routeForPath(r.URL.Path, patterns)),
				logx.Field("status", statusWriter.status),
				logx.Field("duration_ms", time.Since(started).Milliseconds()),
			)
		})
	}
}

func compileRoutePatterns(patterns []string) []routePattern {
	compiled := make([]routePattern, 0, len(patterns))
	for _, pattern := range patterns {
		compiled = append(compiled, routePattern{raw: pattern, segments: splitPath(pattern)})
	}
	return compiled
}

func routeForPath(path string, patterns []routePattern) string {
	pathSegments := splitPath(path)
	for _, pattern := range patterns {
		if len(pattern.segments) != len(pathSegments) {
			continue
		}
		matched := true
		for i := range pattern.segments {
			if strings.HasPrefix(pattern.segments[i], ":") {
				continue
			}
			if pattern.segments[i] != pathSegments[i] {
				matched = false
				break
			}
		}
		if matched {
			return pattern.raw
		}
	}
	return "unmatched"
}

func splitPath(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func Log(ctx context.Context, level Level, message string, fields ...logx.LogField) {
	logger := logx.WithContext(ctx)
	if level == LevelError {
		logger.Errorw(message, fields...)
		return
	}
	logger.Infow(message, fields...)
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *flushingStatusResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.flusher.Flush()
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func validRequestID(value string) bool {
	if len(value) < 8 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '-', char == '_', char == '.':
		default:
			return false
		}
	}
	return true
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
}
