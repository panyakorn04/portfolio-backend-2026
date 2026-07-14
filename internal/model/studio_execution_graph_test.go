package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStudioGraphExecutionPersistenceContract(t *testing.T) {
	t.Parallel()
	calls := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		calls[r.URL.Path] = body
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/rpc/enqueueStudioGraphExecution":
			_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Flow","status":"queued","triggerNodeId":"manual","targetNodeId":"request","mode":"through-target","source":"node-test","startedAt":"2026-01-01T00:00:00Z","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","durationMs":0,"cost":0}]`))
		case "/rest/v1/rpc/claimStudioGraphExecution":
			_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Flow","status":"running","triggerNodeId":"manual","targetNodeId":"request","mode":"through-target","source":"node-test","leaseOwner":"worker-1","startedAt":"2026-01-01T00:00:00Z","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","durationMs":0,"cost":0}]`))
		case "/rest/v1/rpc/startStudioExecutionStage", "/rest/v1/rpc/finishStudioExecutionStage", "/rest/v1/rpc/finishStudioGraphExecution":
			_, _ = w.Write([]byte(`true`))
		case "/rest/v1/rpc/cancelStudioGraphExecution":
			_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Flow","status":"cancelled","startedAt":"2026-01-01T00:00:00Z","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","durationMs":1,"cost":0}]`))
		case "/rest/v1/rpc/deleteStudioWorkflowIfIdle":
			_, _ = w.Write([]byte(`true`))
		case "/rest/v1/StudioExecution":
			if r.URL.Query().Get("status") != "" {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_, _ = w.Write([]byte(`[{"id":"run-1","workflowId":"wf-1","workflow":"Flow","status":"completed","startedAt":"2026-01-01T00:00:00Z","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:01Z","durationMs":1000,"cost":0}]`))
			}
		case "/rest/v1/StudioExecutionStage":
			_, _ = w.Write([]byte(`[{"executionId":"run-1","position":0,"nodeId":"manual","nodeType":"manual","name":"Manual","status":"completed","detail":"Completed","metadata":{},"input":[],"output":[{"json":{"trigger":"manual"}}],"durationMs":1,"updatedAt":"2026-01-01T00:00:00Z"}]`))
		default:
			http.Error(w, "unexpected route", http.StatusNotFound)
		}
	}))
	defer server.Close()

	model := NewStudioModel(NewSupabaseREST(server.URL, "service-key"))
	ctx := context.Background()
	run, err := model.EnqueueGraphExecution(ctx, StudioGraphExecutionInput{
		WorkflowID: "wf-1", WorkflowName: "Flow", WorkflowUpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		TriggerNodeID: "manual", TargetNodeID: "request", Mode: "through-target", Source: "node-test",
		SourceKey: "wf-1:node-test:once", InitialInput: []map[string]any{},
		Path: []StudioExecutionPathNode{{ID: "manual", Type: "manual", Label: "Manual"}, {ID: "request", Type: "http-request", Label: "HTTP Request"}},
	})
	if err != nil || run.Status != "queued" {
		t.Fatalf("enqueue failed: run=%#v err=%v", run, err)
	}
	enqueueBody := calls["/rest/v1/rpc/enqueueStudioGraphExecution"]
	if enqueueBody["sourceKey"] != "wf-1:node-test:once" || enqueueBody["initialInput"] == nil {
		t.Fatalf("enqueue contract lost idempotency or input: %#v", enqueueBody)
	}

	claimed, err := model.ClaimGraphExecution(ctx, "worker-1")
	if err != nil || claimed == nil || claimed.LeaseOwner != "worker-1" {
		t.Fatalf("claim failed: claimed=%#v err=%v", claimed, err)
	}
	if changed, err := model.StartExecutionStage(ctx, "run-1", 0, "worker-1", []map[string]any{}); err != nil || !changed {
		t.Fatalf("start stage failed: changed=%v err=%v", changed, err)
	}
	if changed, err := model.FinishExecutionStage(ctx, "run-1", 0, "worker-1", "completed", []map[string]any{{"json": map[string]any{"ok": true}}}, "", "", "Completed"); err != nil || !changed {
		t.Fatalf("finish stage failed: changed=%v err=%v", changed, err)
	}
	if changed, err := model.FinishGraphExecution(ctx, "run-1", "worker-1", "completed", "", ""); err != nil || !changed {
		t.Fatalf("finish execution failed: changed=%v err=%v", changed, err)
	}
	cancelled, err := model.CancelGraphExecution(ctx, "run-1")
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("cancel failed: item=%#v err=%v", cancelled, err)
	}
	detail, err := model.ExecutionDetail(ctx, "run-1")
	if err != nil || detail == nil || len(detail.Stages) != 1 || detail.Stages[0].NodeID != "manual" {
		t.Fatalf("detail failed: detail=%#v err=%v", detail, err)
	}
	if selectValue := calls["/rest/v1/StudioExecutionStage"]; selectValue != nil {
		t.Fatalf("GET stage request unexpectedly had a JSON body: %#v", selectValue)
	}
	active, err := model.HasActiveExecutions(ctx, "wf-1")
	if err != nil || active {
		t.Fatalf("active execution probe failed: active=%v err=%v", active, err)
	}
	deleted, err := model.DeleteWorkflowIfIdle(ctx, "wf-1")
	if err != nil || !deleted {
		t.Fatalf("atomic workflow delete failed: deleted=%v err=%v", deleted, err)
	}
	if calls["/rest/v1/rpc/deleteStudioWorkflowIfIdle"]["workflowId"] != "wf-1" {
		t.Fatalf("delete RPC lost workflow id: %#v", calls["/rest/v1/rpc/deleteStudioWorkflowIfIdle"])
	}
	if !strings.Contains(server.URL, "http") {
		t.Fatal("test server was not initialized")
	}
}
