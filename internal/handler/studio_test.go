package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestValidateStudioWorkflowAndTransitions(t *testing.T) {
	_, message := validateStudioWorkflow(studioWorkflowPayload{Name: "x", Category: "Content", Status: "active", Nodes: []string{"A"}})
	if message == "" {
		t.Fatal("expected name length validation")
	}
	input, message := validateStudioWorkflow(studioWorkflowPayload{Name: "Flow", Category: "Content", Status: "active", Nodes: []string{" A "}})
	if message != "" || input.Nodes[0] != "A" {
		t.Fatalf("input=%#v message=%q", input, message)
	}
	if !executionTransitions["failed"]["running"] || executionTransitions["completed"]["running"] {
		t.Fatal("retry transition rules are incorrect")
	}
}
