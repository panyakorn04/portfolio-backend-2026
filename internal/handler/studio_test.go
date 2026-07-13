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

	"github.com/zeromicro/go-zero/rest/pathvar"
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

func TestAdminExecuteStudioManualTriggerReturnsJSONOutput(t *testing.T) {
	var auditBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/StudioWorkflow":
			_, _ = w.Write([]byte(`[{"id":"wf-1","name":"Manual Flow","description":"Demo","category":"Ops","status":"draft","runs":0,"success":0,"nodes":["Manual","Transform"],"definition":{"version":1,"nodes":[{"id":"manual","type":"manual","kind":"trigger","label":"Manual","position":{"x":0,"y":0},"config":{"enabled":true}},{"id":"transform","type":"transform","kind":"action","label":"Transform","position":{"x":200,"y":0},"config":{}}],"edges":[{"id":"edge-manual-transform","source":"manual","target":"transform"}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		case "/rest/v1/StudioAuditLog":
			_ = json.NewDecoder(r.Body).Decode(&auditBody)
			_, _ = w.Write([]byte(`[{"id":"audit-1","actorType":"bearer","actorLabel":"admin bearer","action":"node.execute","resourceType":"workflow-node","resourceId":"manual","metadata":{},"createdAt":"2026-07-13T12:00:00Z"}]`))
		default:
			t.Fatalf("unexpected persistence path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	s := &svc.ServiceContext{Config: config.Config{AdminApiToken: "test-token"}, HasDatabse: true, Studio: model.NewStudioModel(model.NewSupabaseREST(server.URL, "key"))}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/studio/workflows/wf-1/nodes/manual/execute", nil)
	req = pathvar.WithVars(req, map[string]string{"id": "wf-1", "nodeId": "manual"})
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	AdminExecuteStudioTriggerHandler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"trigger":"manual"`) || auditBody["action"] != "node.execute" {
		t.Fatalf("status=%d body=%s audit=%#v", rec.Code, rec.Body.String(), auditBody)
	}
}

func TestAdminExecuteStudioScheduleTriggerReturnsJSONOutput(t *testing.T) {
	var auditBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/StudioWorkflow":
			_, _ = w.Write([]byte(`[{"id":"wf-1","name":"Scheduled Flow","description":"Demo","category":"Ops","status":"draft","runs":0,"success":0,"nodes":["Schedule","Transform"],"definition":{"version":1,"nodes":[{"id":"schedule","type":"schedule","kind":"trigger","label":"Schedule","position":{"x":0,"y":0},"config":{"enabled":true,"mode":"cron","timezone":"Asia/Bangkok","cronExpression":"0 12,20 * * *","misfirePolicy":"skip"}},{"id":"transform","type":"transform","kind":"action","label":"Transform","position":{"x":200,"y":0},"config":{}}],"edges":[{"id":"edge-schedule-transform","source":"schedule","target":"transform"}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		case "/rest/v1/StudioAuditLog":
			_ = json.NewDecoder(r.Body).Decode(&auditBody)
			_, _ = w.Write([]byte(`[{"id":"audit-1","actorType":"bearer","actorLabel":"admin bearer","action":"node.execute","resourceType":"workflow-node","resourceId":"schedule","metadata":{},"createdAt":"2026-07-13T12:00:00Z"}]`))
		default:
			t.Fatalf("unexpected persistence path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	s := &svc.ServiceContext{Config: config.Config{AdminApiToken: "test-token"}, HasDatabse: true, Studio: model.NewStudioModel(model.NewSupabaseREST(server.URL, "key"))}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/studio/workflows/wf-1/nodes/schedule/execute", nil)
	req = pathvar.WithVars(req, map[string]string{"id": "wf-1", "nodeId": "schedule"})
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	AdminExecuteStudioTriggerHandler(s).ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, `"trigger":"schedule"`) || !strings.Contains(body, `"timezone":"Asia/Bangkok"`) || auditBody["action"] != "node.execute" {
		t.Fatalf("status=%d body=%s audit=%#v", rec.Code, body, auditBody)
	}
}

func TestAdminExecuteStudioTriggerRejectsInvalidPersistedDefinition(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"wf-1","name":"Invalid Draft","description":"Demo","category":"Ops","status":"draft","runs":0,"success":0,"nodes":["Schedule"],"definition":{"version":1,"nodes":[{"id":"schedule","type":"schedule","kind":"trigger","label":"Schedule","position":{"x":0,"y":0},"config":{"enabled":true,"mode":"cron","timezone":"Invalid/Zone","cronExpression":"0 9 * * *","misfirePolicy":"skip"}}],"edges":[]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()
	s := &svc.ServiceContext{Config: config.Config{AdminApiToken: "test-token"}, HasDatabse: true, Studio: model.NewStudioModel(model.NewSupabaseREST(server.URL, "key"))}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/studio/workflows/wf-1/nodes/schedule/execute", nil)
	req = pathvar.WithVars(req, map[string]string{"id": "wf-1", "nodeId": "schedule"})
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	AdminExecuteStudioTriggerHandler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "valid and complete") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
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

func TestValidateWorkflowDefinitionAcceptsMultipleTriggerRoots(t *testing.T) {
	definition := &model.StudioWorkflowDefinition{Version: 1, Nodes: []model.StudioWorkflowNode{
		{ID: "schedule", Type: "schedule", Kind: "trigger", Label: "Schedule", Position: model.StudioPosition{X: 0, Y: 0}, Config: map[string]any{"enabled": true, "mode": "daily", "timezone": "Asia/Bangkok", "time": "09:00", "misfirePolicy": "skip"}},
		{ID: "manual", Type: "manual", Kind: "trigger", Label: "Manual", Position: model.StudioPosition{X: 0, Y: 140}, Config: map[string]any{"enabled": true}},
		{ID: "transform", Type: "transform", Kind: "action", Label: "Transform", Position: model.StudioPosition{X: 200}, Config: map[string]any{}},
	}, Edges: []model.StudioWorkflowEdge{
		{ID: "edge-schedule-transform", Source: "schedule", Target: "transform"},
		{ID: "edge-manual-transform", Source: "manual", Target: "transform"},
	}}
	labels, message := validateWorkflowDefinition(definition, true)
	if message != "" || len(labels) != 3 || labels[0] != "Schedule" || labels[1] != "Manual" {
		t.Fatalf("expected valid multi-trigger graph: labels=%v message=%q", labels, message)
	}
}

func TestValidateWorkflowDefinitionRejectsDisconnectedGraphsAndUnknownParameters(t *testing.T) {
	definition := &model.StudioWorkflowDefinition{Version: 1, Nodes: []model.StudioWorkflowNode{
		{ID: "node-0", Type: "manual", Kind: "trigger", Label: "Manual", Position: model.StudioPosition{X: 0}, Config: map[string]any{"enabled": true}},
		{ID: "node-1", Type: "transform", Kind: "action", Label: "Transform", Position: model.StudioPosition{X: 200}, Config: map[string]any{}},
	}}
	if _, message := validateWorkflowDefinition(definition, true); message == "" {
		t.Fatal("expected disconnected graph validation")
	}
	definition.Edges = []model.StudioWorkflowEdge{{ID: "edge-node-0-node-1", Source: "node-0", Target: "node-1"}}
	if labels, message := validateWorkflowDefinition(definition, true); message != "" || len(labels) != 2 {
		t.Fatalf("expected valid linear graph: labels=%v message=%q", labels, message)
	}
	definition.Nodes[1].Config["authorization"] = "Bearer must-not-persist"
	if _, message := validateWorkflowDefinition(definition, true); message == "" {
		t.Fatal("expected strict action config allowlist validation")
	}
}

func TestValidateCronExpression(t *testing.T) {
	for _, expression := range []string{"0 9 * * *", "*/15 8-17 * * 1-5", "0,30 9 * 1,6 1"} {
		if !validCronExpression(expression) {
			t.Fatalf("expected valid cron expression: %q", expression)
		}
	}
	for _, expression := range []string{"foo bar baz qux quux", "60 9 * * *", "0 24 * * *", "0 9 * 13 *", "0 9 * * 7", "*/0 * * * *"} {
		if validCronExpression(expression) {
			t.Fatalf("expected invalid cron expression: %q", expression)
		}
	}
}
