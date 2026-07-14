package model

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

const StudioExecutionLeaseSeconds = 90

type StudioExecutionPathNode struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Label string `json:"label"`
}

type StudioGraphExecutionInput struct {
	WorkflowID         string
	WorkflowName       string
	WorkflowUpdatedAt  time.Time
	TriggerNodeID      string
	TargetNodeID       string
	Mode               string
	Source             string
	SourceKey          string
	RetryOfExecutionID string
	InitialInput       []map[string]any
	Path               []StudioExecutionPathNode
}

type StudioExecutionDetail struct {
	Execution StudioExecution        `json:"execution"`
	Stages    []StudioExecutionStage `json:"stages"`
}

func (m *StudioModel) EnqueueGraphExecution(ctx context.Context, input StudioGraphExecutionInput) (*StudioExecution, error) {
	now := time.Now().UTC()
	body := map[string]any{
		"executionId": newID(), "workflowId": input.WorkflowID, "workflowName": input.WorkflowName,
		"workflowUpdatedAt": input.WorkflowUpdatedAt, "triggerNodeId": input.TriggerNodeID,
		"targetNodeId": input.TargetNodeID, "executionMode": input.Mode, "executionSource": input.Source,
		"sourceKey": nullableStudioString(input.SourceKey), "retryOfExecutionId": nullableStudioString(input.RetryOfExecutionID),
		"initialInput": input.InitialInput, "pathNodes": input.Path, "occurredAt": now,
	}
	var rows []studioExecutionRow
	if _, err := m.api.request(ctx, http.MethodPost, "rpc/enqueueStudioGraphExecution", nil, body, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	item := executionFromRow(rows[0])
	return &item, nil
}

func nullableStudioString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (m *StudioModel) ClaimGraphExecution(ctx context.Context, workerID string) (*StudioExecution, error) {
	body := map[string]any{"workerId": workerID, "leaseSeconds": StudioExecutionLeaseSeconds, "occurredAt": time.Now().UTC()}
	var rows []studioExecutionRow
	if _, err := m.api.request(ctx, http.MethodPost, "rpc/claimStudioGraphExecution", nil, body, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	item := executionFromRow(rows[0])
	return &item, nil
}

func (m *StudioModel) StartExecutionStage(ctx context.Context, executionID string, position int, workerID string, input []map[string]any) (bool, error) {
	body := map[string]any{
		"executionId": executionID, "stagePosition": position, "workerId": workerID,
		"stageInput": input, "leaseSeconds": StudioExecutionLeaseSeconds, "occurredAt": time.Now().UTC(),
	}
	var changed bool
	_, err := m.api.request(ctx, http.MethodPost, "rpc/startStudioExecutionStage", nil, body, "", &changed)
	return changed, err
}

func (m *StudioModel) FinishExecutionStage(ctx context.Context, executionID string, position int, workerID, status string, output []map[string]any, errorCode, errorMessage, detail string) (bool, error) {
	body := map[string]any{
		"executionId": executionID, "stagePosition": position, "workerId": workerID, "stageStatus": status,
		"stageOutput": output, "stageErrorCode": nullableStudioString(errorCode),
		"stageErrorMessage": nullableStudioString(errorMessage), "stageDetail": detail, "occurredAt": time.Now().UTC(),
	}
	var changed bool
	_, err := m.api.request(ctx, http.MethodPost, "rpc/finishStudioExecutionStage", nil, body, "", &changed)
	return changed, err
}

func (m *StudioModel) FinishGraphExecution(ctx context.Context, executionID, workerID, status, errorCode, errorMessage string) (bool, error) {
	body := map[string]any{
		"executionId": executionID, "workerId": workerID, "executionStatus": status,
		"executionErrorCode": nullableStudioString(errorCode), "executionErrorMessage": nullableStudioString(errorMessage),
		"occurredAt": time.Now().UTC(),
	}
	var changed bool
	_, err := m.api.request(ctx, http.MethodPost, "rpc/finishStudioGraphExecution", nil, body, "", &changed)
	return changed, err
}

func (m *StudioModel) CancelGraphExecution(ctx context.Context, executionID string) (*StudioExecution, error) {
	body := map[string]any{"executionId": executionID, "occurredAt": time.Now().UTC()}
	var rows []studioExecutionRow
	if _, err := m.api.request(ctx, http.MethodPost, "rpc/cancelStudioGraphExecution", nil, body, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	item := executionFromRow(rows[0])
	return &item, nil
}

func (m *StudioModel) ExecutionDetail(ctx context.Context, executionID string) (*StudioExecutionDetail, error) {
	execution, err := m.FindExecution(ctx, executionID)
	if err != nil || execution == nil {
		return nil, err
	}
	stages, err := m.ListExecutionStages(ctx, executionID)
	if err != nil {
		return nil, err
	}
	return &StudioExecutionDetail{Execution: *execution, Stages: stages}, nil
}

func (m *StudioModel) HasActiveExecutions(ctx context.Context, workflowID string) (bool, error) {
	values := url.Values{
		"workflowId": {"eq." + workflowID},
		"status":     {"in.(queued,running,cancellation_requested)"},
		"select":     {"id"},
		"limit":      {"1"},
	}
	var rows []struct {
		ID string `json:"id"`
	}
	if _, err := m.api.request(ctx, http.MethodGet, "StudioExecution", values, nil, "", &rows); err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (m *StudioModel) DeleteWorkflowIfIdle(ctx context.Context, workflowID string) (bool, error) {
	var deleted bool
	_, err := m.api.request(ctx, http.MethodPost, "rpc/deleteStudioWorkflowIfIdle", nil, map[string]any{"workflowId": workflowID}, "", &deleted)
	return deleted, err
}
