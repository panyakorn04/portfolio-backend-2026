package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStudioOverviewHandlerReturnsPublicReadModel(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/studio/overview", nil)
	recorder := httptest.NewRecorder()

	StudioOverviewHandler()(recorder, req)

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
}
