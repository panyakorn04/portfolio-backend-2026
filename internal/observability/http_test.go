package observability

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
	resthandler "github.com/zeromicro/go-zero/rest/handler"
)

type capturedEvent struct {
	level   Level
	message string
	fields  map[string]any
}

type nonFlushingWriter struct {
	recorder *httptest.ResponseRecorder
}

func (w *nonFlushingWriter) Header() http.Header            { return w.recorder.Header() }
func (w *nonFlushingWriter) Write(body []byte) (int, error) { return w.recorder.Write(body) }
func (w *nonFlushingWriter) WriteHeader(status int)         { w.recorder.WriteHeader(status) }

func captureEvents() (LogFunc, *[]capturedEvent) {
	events := make([]capturedEvent, 0, 1)
	return func(_ context.Context, level Level, message string, fields ...logx.LogField) {
		values := make(map[string]any, len(fields))
		for _, field := range fields {
			values[field.Key] = field.Value
		}
		events = append(events, capturedEvent{level: level, message: message, fields: values})
	}, &events
}

func TestHTTPMiddlewareUsesSafeRequestIDAndLogsOnlyMetadata(t *testing.T) {
	logFn, events := captureEvents()
	middleware := HTTPMiddleware(logFn, "/api/admin/sessions/:id")

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != "request-123" {
			t.Fatalf("request id in context = %q", got)
		}
		w.WriteHeader(http.StatusAccepted)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/admin/sessions/session-secret?token=query-secret", strings.NewReader(`{"password":"body-secret"}`))
	request.Header.Set(RequestIDHeader, "request-123")
	request.Header.Set("Authorization", "Bearer header-secret")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(RequestIDHeader); got != "request-123" {
		t.Fatalf("response request id = %q", got)
	}
	if len(*events) != 1 {
		t.Fatalf("events = %d, want 1", len(*events))
	}
	event := (*events)[0]
	if event.level != LevelInfo || event.message != "http request completed" {
		t.Fatalf("event = %#v", event)
	}
	if got := event.fields["event"]; got != "http.request.completed" {
		t.Fatalf("event field = %#v", got)
	}
	if got := event.fields["request_id"]; got != "request-123" {
		t.Fatalf("request_id = %#v", got)
	}
	if got := event.fields["method"]; got != http.MethodPost {
		t.Fatalf("method = %#v", got)
	}
	if got := event.fields["route"]; got != "/api/admin/sessions/:id" {
		t.Fatalf("route = %#v", got)
	}
	if got := event.fields["status"]; got != http.StatusAccepted {
		t.Fatalf("status = %#v", got)
	}
	if _, ok := event.fields["duration_ms"]; !ok {
		t.Fatal("duration_ms is missing")
	}

	for key, value := range event.fields {
		text := key + "=" + strings.TrimSpace(toString(value))
		for _, secret := range []string{"session-secret", "query-secret", "body-secret", "header-secret", "Authorization", "password", "token"} {
			if strings.Contains(text, secret) {
				t.Fatalf("sensitive value %q leaked through field %q", secret, key)
			}
		}
	}
}

func TestHTTPMiddlewareKeepsFirstResponseStatus(t *testing.T) {
	logFn, events := captureEvents()
	handler := HTTPMiddleware(logFn)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/articles", nil))

	if len(*events) != 1 {
		t.Fatalf("events = %d, want 1", len(*events))
	}
	if got := (*events)[0].fields["status"]; got != http.StatusCreated {
		t.Fatalf("status = %#v, want %d", got, http.StatusCreated)
	}
	if (*events)[0].level != LevelInfo {
		t.Fatalf("level = %q, want %q", (*events)[0].level, LevelInfo)
	}
}

func TestHTTPMiddlewareReplacesInvalidRequestIDAndPreservesFlusher(t *testing.T) {
	logFn, events := captureEvents()
	middleware := HTTPMiddleware(logFn)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatal("wrapped response writer does not implement http.Flusher")
		}
		if got := RequestIDFromContext(r.Context()); got == "bad id with spaces" || got == "" {
			t.Fatalf("request id was not regenerated: %q", got)
		}
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	request.Header.Set(RequestIDHeader, "bad id with spaces")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if got := recorder.Header().Get(RequestIDHeader); got == "bad id with spaces" || got == "" {
		t.Fatalf("response request id was not regenerated: %q", got)
	}
	if len(*events) != 1 || (*events)[0].level != LevelError {
		t.Fatalf("events = %#v", *events)
	}
}

func TestHTTPMiddlewareDoesNotClaimUnsupportedFlusher(t *testing.T) {
	handler := HTTPMiddleware(func(context.Context, Level, string, ...logx.LogField) {})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, ok := w.(http.Flusher); ok {
			t.Fatal("wrapped writer unexpectedly implements http.Flusher")
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	writer := &nonFlushingWriter{recorder: httptest.NewRecorder()}
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/api/health", nil))
}

func TestHTTPRouterLogsNativeTimeoutStatus(t *testing.T) {
	logFn, events := captureEvents()
	requestRouter := NewHTTPRouter(logFn)
	requestRouter.SetRoutePatterns([]string{"/slow"})
	release := make(chan struct{})
	timeoutHandler := resthandler.TimeoutHandler(time.Millisecond)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		<-release
	}))
	if err := requestRouter.Handle(http.MethodGet, "/slow", timeoutHandler); err != nil {
		t.Fatalf("register route: %v", err)
	}

	recorder := httptest.NewRecorder()
	requestRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/slow", nil))
	close(release)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if len(*events) != 1 || (*events)[0].fields["status"] != http.StatusServiceUnavailable || (*events)[0].level != LevelError {
		t.Fatalf("events = %#v", *events)
	}
}

func TestHTTPRouterLogsNativeRecoveredPanic(t *testing.T) {
	logFn, events := captureEvents()
	requestRouter := NewHTTPRouter(logFn)
	requestRouter.SetRoutePatterns([]string{"/panic"})
	panicHandler := resthandler.RecoverHandler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	if err := requestRouter.Handle(http.MethodGet, "/panic", panicHandler); err != nil {
		t.Fatalf("register route: %v", err)
	}

	recorder := httptest.NewRecorder()
	requestRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if len(*events) != 1 || (*events)[0].fields["status"] != http.StatusInternalServerError || (*events)[0].level != LevelError {
		t.Fatalf("events = %#v", *events)
	}
}

func TestHTTPRouterLogsNotFoundAndMethodNotAllowed(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantRoute string
	}{
		{name: "not found", method: http.MethodGet, path: "/missing/visitor-123", wantCode: http.StatusNotFound, wantRoute: "unmatched"},
		{name: "cors method not allowed", method: http.MethodPost, path: "/known", wantCode: http.StatusNotFound, wantRoute: "/known"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logFn, events := captureEvents()
			requestRouter := NewHTTPRouter(logFn)
			requestRouter.SetRoutePatterns([]string{"/known"})
			if err := requestRouter.Handle(http.MethodGet, "/known", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})); err != nil {
				t.Fatalf("register route: %v", err)
			}

			recorder := httptest.NewRecorder()
			requestRouter.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))

			if recorder.Code != test.wantCode {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantCode)
			}
			if len(*events) != 1 || (*events)[0].fields["status"] != test.wantCode || (*events)[0].fields["route"] != test.wantRoute {
				t.Fatalf("events = %#v", *events)
			}
			if strings.Contains(fmt.Sprint((*events)[0].fields), "visitor-123") {
				t.Fatal("unmatched path leaked into request log")
			}
		})
	}
}

func TestHTTPRouterLogsCORSPreflightWithRequestID(t *testing.T) {
	logFn, events := captureEvents()
	requestRouter := NewHTTPRouter(logFn, "https://panyakorn.com")
	requestRouter.SetRoutePatterns([]string{"/api/health"})

	request := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	request.Header.Set("Origin", "https://panyakorn.com")
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	recorder := httptest.NewRecorder()
	requestRouter.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if recorder.Header().Get(RequestIDHeader) == "" {
		t.Fatal("preflight response is missing request ID")
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Allow-Headers"), RequestIDHeader) {
		t.Fatalf("allowed headers = %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}
	if !strings.Contains(recorder.Header().Get("Access-Control-Expose-Headers"), RequestIDHeader) {
		t.Fatalf("exposed headers = %q", recorder.Header().Get("Access-Control-Expose-Headers"))
	}
	if len(*events) != 1 || (*events)[0].fields["status"] != http.StatusNoContent || (*events)[0].fields["route"] != "/api/health" {
		t.Fatalf("events = %#v", *events)
	}
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return "non-string"
}
