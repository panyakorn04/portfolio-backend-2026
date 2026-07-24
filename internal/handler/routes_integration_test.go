package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func newRouteIntegrationServer(t *testing.T) *rest.Server {
	t.Helper()
	server := rest.MustNewServer(rest.RestConf{Host: "127.0.0.1", Port: 0})
	t.Cleanup(server.Stop)
	RegisterHandlers(server, &svc.ServiceContext{Config: config.Config{
		AdminApiToken:              "test",
		PortfolioChatVisitorSecret: strings.Repeat("x", 32),
	}})
	return server
}

func serveRoute(server *rest.Server, method, target, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestCriticalPublicRoutesAreRegisteredThroughProductionRouter(t *testing.T) {
	server := newRouteIntegrationServer(t)

	health := serveRoute(server, http.MethodGet, "/api/health", "")
	if health.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", health.Code, health.Body.String())
	}

	ready := serveRoute(server, http.MethodGet, "/api/ready", "")
	if ready.Code == http.StatusNotFound || ready.Code == http.StatusMethodNotAllowed {
		t.Fatalf("readiness route is not wired: status = %d", ready.Code)
	}

	handoff := serveRoute(server, http.MethodPost, "/api/portfolio/assistant/sessions/session-1/request-human", "{}")
	if handoff.Code == http.StatusNotFound || handoff.Code == http.StatusMethodNotAllowed {
		t.Fatalf("handoff route is not wired: status = %d", handoff.Code)
	}
}

func TestAdminChatRoutesEnforceAuthenticationThroughProductionRouter(t *testing.T) {
	server := newRouteIntegrationServer(t)
	for _, item := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/admin/chat/sessions", ""},
		{http.MethodGet, "/api/admin/chat/sessions/session-1", ""},
		{http.MethodPost, "/api/admin/chat/sessions/session-1/reply", `{"message":"hello"}`},
		{http.MethodPatch, "/api/admin/chat/sessions/session-1", `{"status":"closed"}`},
	} {
		response := serveRoute(server, item.method, item.path, item.body)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want 401", item.method, item.path, response.Code)
		}
	}
}

func TestStrictJSONDecoderIsEnforcedThroughProductionRouter(t *testing.T) {
	server := newRouteIntegrationServer(t)
	response := serveRoute(
		server,
		http.MethodPost,
		"/api/contact",
		`{"name":"Test","email":"test@example.com","subject":"Hi","message":"Hello"} {}`,
	)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("multiple JSON values status = %d, body = %s", response.Code, response.Body.String())
	}
}
