package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

func TestStudioOverviewHandlerReturnsPublicReadModel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/studio/overview", nil)
	recorder := httptest.NewRecorder()

	StudioOverviewHandler(nil)(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Workflows  []map[string]any `json:"workflows"`
			Executions []map[string]any `json:"executions"`
			Stages     []map[string]any `json:"stages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || len(body.Data.Workflows) == 0 || len(body.Data.Executions) == 0 || len(body.Data.Stages) == 0 {
		t.Fatalf("expected populated public read model, got %#v", body)
	}
	if _, ok := body.Data.Workflows[0]["updated"]; !ok {
		t.Fatal("public workflow contract must include updated")
	}
	if _, ok := body.Data.Executions[0]["duration"]; !ok {
		t.Fatal("public execution contract must include duration")
	}
}

func TestStudioOverviewHandlerFallsBackWhenTablesUnavailable(t *testing.T) {
	svcCtx := &svc.ServiceContext{Studio: model.NewStudioModel(model.NewSupabaseREST("http://127.0.0.1:1", "key")), HasDatabse: true}
	rec := httptest.NewRecorder()
	StudioOverviewHandler(svcCtx).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/studio/overview", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "wf-content") {
		t.Fatalf("expected safe seed fallback, status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminCreateStudioExecutionPersistsAndAuditsActiveWorkflow(t *testing.T) {
	var executionBody, auditBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/StudioWorkflow":
			_, _ = w.Write([]byte(`[{"id":"wf-1","name":"Active Flow","description":"Demo","category":"Ops","status":"active","runs":0,"success":0,"nodes":["Start"],"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		case "/rest/v1/rpc/createStudioExecutionWithStages":
			_ = json.NewDecoder(r.Body).Decode(&executionBody)
			_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Active Flow","status":"running","startedAt":"2026-07-11T10:00:00Z","durationMs":0,"cost":0,"createdAt":"2026-07-11T10:00:00Z","updatedAt":"2026-07-11T10:00:00Z"}]`))
		case "/rest/v1/StudioExecutionStage":
			w.WriteHeader(http.StatusNoContent)
		case "/rest/v1/StudioAuditLog":
			_ = json.NewDecoder(r.Body).Decode(&auditBody)
			_, _ = w.Write([]byte(`[{"id":"audit-1","actorType":"bearer","actorId":null,"actorLabel":"admin bearer","action":"execution.create","resourceType":"execution","resourceId":"run-1","fromStatus":null,"toStatus":"running","metadata":{},"createdAt":"2026-07-11T10:00:00Z"}]`))
		default:
			t.Fatalf("unexpected persistence path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	s := &svc.ServiceContext{Config: config.Config{AdminApiToken: "test-token"}, HasDatabse: true, Studio: model.NewStudioModel(model.NewSupabaseREST(server.URL, "key"))}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/studio/executions", strings.NewReader(`{"workflowId":"wf-1"}`))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	AdminCreateStudioExecutionHandler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if executionBody["workflowName"] != "Active Flow" || len(executionBody["nodes"].([]any)) != 1 || auditBody["action"] != "execution.create" || auditBody["resourceId"] != "run-1" {
		t.Fatalf("execution=%#v audit=%#v", executionBody, auditBody)
	}
}

func TestValidateStudioWorkflowAndTransitions(t *testing.T) {
	_, message := validateStudioWorkflow(studioWorkflowPayload{Name: "x", Category: "Content", Status: "active", Nodes: []string{"A"}})
	if message == "" {
		t.Fatal("expected name length validation")
	}
	input, message := validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{" A "}})
	if message != "" || input.Nodes[0] != "A" {
		t.Fatalf("input=%#v message=%q", input, message)
	}

	definition := &model.StudioWorkflowDefinition{Version: 1, Nodes: []model.StudioWorkflowNode{{
		ID: "node-0", Type: "schedule", Kind: "trigger", Label: "Schedule",
		Position: model.StudioPosition{X: 0, Y: 0},
		Config:   map[string]any{"enabled": true, "mode": "daily", "timezone": "Asia/Bangkok", "time": "09:00", "misfirePolicy": "skip"},
	}}}
	input, message = validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{"Schedule"}, Definition: definition})
	if message != "" || input.Definition == nil || input.Nodes[0] != "Schedule" {
		t.Fatalf("definition input=%#v message=%q", input, message)
	}
	definition.Nodes[0].Config["enabled"] = false
	if _, message = validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{"Schedule"}, Definition: definition}); message != "" {
		t.Fatalf("disabled schedule should remain a valid configuration: %s", message)
	}
	definition.Nodes[0].Config["enabled"] = true
	definition.Nodes = append(definition.Nodes, model.StudioWorkflowNode{ID: "node-1", Type: "transform", Kind: "action", Label: "Transform", Position: model.StudioPosition{X: -100, Y: 0}, Config: map[string]any{}})
	if _, message = validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{"Transform", "Schedule"}, Definition: definition}); message == "" {
		t.Fatal("expected left-most trigger validation")
	}
	definition.Nodes = definition.Nodes[:1]

	definition.Nodes[0].Config["timezone"] = "Not/AZone"
	if _, message = validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{"Schedule"}, Definition: definition}); message == "" {
		t.Fatal("expected invalid timezone validation")
	}
	definition.Nodes[0].Config["timezone"] = "Asia/Bangkok"
	definition.Nodes[0].Config["auth"] = map[string]any{"apiToken": "must-not-persist"}
	if _, message = validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{"Schedule"}, Definition: definition}); message == "" {
		t.Fatal("expected nested credential validation")
	}
	if !executionTransitions["failed"]["running"] || executionTransitions["completed"]["running"] {
		t.Fatal("retry transition rules are incorrect")
	}
}
