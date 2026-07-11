package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStudioAuditCreateAndListRESTBoundary(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/StudioAuditLog" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			if r.Header.Get("Prefer") != "return=representation" {
				t.Fatalf("Prefer=%q", r.Header.Get("Prefer"))
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["action"] != "workflow.create" || strings.Contains(r.Header.Get("Authorization"), "secret") {
				t.Fatalf("body=%#v", body)
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{body})
			return
		}
		if r.URL.Query().Get("order") != "createdAt.desc" || r.URL.Query().Get("limit") != "50" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	m := NewStudioModel(NewSupabaseREST(server.URL, "key"))
	_, err := m.CreateAudit(context.Background(), StudioAuditInput{ActorType: "session", ActorID: "u1", ActorLabel: "admin@example.com", Action: "workflow.create", ResourceType: "workflow", ResourceID: "wf1", Metadata: map[string]any{"ip": "192.0.2.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.ListAudits(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}
