package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStudioModelListOverviewAndUpdateExecution(t *testing.T) {
	var patched map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/StudioWorkflow":
			_, _ = w.Write([]byte(`[{"id":"wf-1","name":"Flow","description":"Demo","category":"Content","status":"active","runs":1,"success":100,"nodes":["A"],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		case "/rest/v1/StudioExecution":
			if r.Method == http.MethodPatch {
				if err := json.NewDecoder(r.Body).Decode(&patched); err != nil {
					t.Fatal(err)
				}
				_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Flow","status":"paused","startedAt":"2026-01-01T00:00:00Z","durationMs":12,"cost":0.1,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Flow","status":"running","startedAt":"2026-01-01T00:00:00Z","durationMs":12,"cost":0.1,"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	m := NewStudioModel(NewSupabaseREST(server.URL, "key"))
	workflows, executions, err := m.Overview(context.Background())
	if err != nil || len(workflows) != 1 || len(executions) != 1 {
		t.Fatalf("overview = %#v %#v, %v", workflows, executions, err)
	}
	got, err := m.TransitionExecution(context.Background(), "run-1", "paused")
	if err != nil || got.Status != "paused" || patched["status"] != "paused" {
		t.Fatalf("transition = %#v, body=%#v, err=%v", got, patched, err)
	}
}
