package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

func TestSessionLogoutFailsClosedWhenRevocationFails(t *testing.T) {
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/rest/v1/AuthSession" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		http.Error(w, "database unavailable", http.StatusInternalServerError)
	}))
	defer supabase.Close()

	rest := model.NewSupabaseREST(supabase.URL, "test-key")
	svcCtx := &svc.ServiceContext{HasDatabse: true, Sessions: model.NewAuthSessionModel(rest)}
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/session", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	SessionLogoutHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("session cookie must remain so revocation can be retried: %v", got)
	}
}

func TestAdminLogoutEverywhereFailsClosedWhenRevocationFails(t *testing.T) {
	now := "2026-07-23T15:00:00Z"
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/v1/AuthSession":
			_, _ = w.Write([]byte(`[{"id":"session-1","userId":"user-1","expiresAt":"2099-01-01T00:00:00Z"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/v1/User":
			_, _ = w.Write([]byte(`[{"id":"user-1","email":"admin@example.com","role":"admin"}]`))
		case r.Method == http.MethodPatch && r.URL.Path == "/rest/v1/AuthSession":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodDelete && r.URL.Path == "/rest/v1/AuthSession":
			http.Error(w, "database unavailable at "+now, http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer supabase.Close()

	rest := model.NewSupabaseREST(supabase.URL, "test-key")
	svcCtx := &svc.ServiceContext{HasDatabse: true, Sessions: model.NewAuthSessionModel(rest)}
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/sessions", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "raw-token"})
	rec := httptest.NewRecorder()

	AdminLogoutEverywhereHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("session cookie must remain so revocation can be retried: %v", got)
	}
}
