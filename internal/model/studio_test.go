package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStudioModelCreateExecutionPersistsRunningRun(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/v1/StudioExecution" || r.URL.Query().Get("select") != "*" || r.Header.Get("Prefer") != "return=representation" {
			t.Fatalf("unexpected request %s %s?%s Prefer=%q", r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Prefer"))
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"run-generated","workflowId":"wf-1","workflow":"Active Flow","status":"running","startedAt":"2026-07-11T10:00:00Z","durationMs":0,"cost":0,"createdAt":"2026-07-11T10:00:00Z","updatedAt":"2026-07-11T10:00:00Z"}]`))
	}))
	defer server.Close()

	got, err := NewStudioModel(NewSupabaseREST(server.URL, "key")).CreateExecution(context.Background(), "wf-1", "Active Flow")
	if err != nil || got.Status != "running" {
		t.Fatalf("execution=%#v err=%v", got, err)
	}
	if body["id"] == "" || body["workflowId"] != "wf-1" || body["workflow"] != "Active Flow" || body["status"] != "running" || body["durationMs"] != float64(0) || body["cost"] != float64(0) {
		t.Fatalf("unexpected body %#v", body)
	}
	for _, field := range []string{"startedAt", "createdAt", "updatedAt"} {
		if _, err := time.Parse(time.RFC3339Nano, body[field].(string)); err != nil {
			t.Fatalf("%s is not a timestamp: %#v", field, body[field])
		}
	}
}

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
