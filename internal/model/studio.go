package model

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type StudioModel struct{ api *SupabaseREST }

func NewStudioModel(api *SupabaseREST) *StudioModel { return &StudioModel{api: api} }

type StudioWorkflow struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	Status      string    `json:"status"`
	Runs        int       `json:"runs"`
	Success     float64   `json:"success"`
	Nodes       []string  `json:"nodes"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type StudioExecution struct {
	ID         string    `json:"id"`
	WorkflowID string    `json:"workflowId"`
	Workflow   string    `json:"workflow"`
	Status     string    `json:"status"`
	StartedAt  time.Time `json:"startedAt"`
	DurationMS int       `json:"durationMs"`
	Cost       float64   `json:"cost"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type StudioWorkflowInput struct {
	Name, Description, Category, Status string
	Nodes                               []string
}

type studioWorkflowRow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Status      string   `json:"status"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
	Runs        int      `json:"runs"`
	Success     float64  `json:"success"`
	Nodes       []string `json:"nodes"`
}
type studioExecutionRow struct {
	ID         string  `json:"id"`
	WorkflowID string  `json:"workflowId"`
	Workflow   string  `json:"workflow"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"startedAt"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
	DurationMS int     `json:"durationMs"`
	Cost       float64 `json:"cost"`
}

func workflowFromRow(r studioWorkflowRow) StudioWorkflow {
	return StudioWorkflow{ID: r.ID, Name: r.Name, Description: r.Description, Category: r.Category, Status: r.Status, Runs: r.Runs, Success: r.Success, Nodes: r.Nodes, CreatedAt: timeFromString(r.CreatedAt), UpdatedAt: timeFromString(r.UpdatedAt)}
}
func executionFromRow(r studioExecutionRow) StudioExecution {
	return StudioExecution{ID: r.ID, WorkflowID: r.WorkflowID, Workflow: r.Workflow, Status: r.Status, StartedAt: timeFromString(r.StartedAt), DurationMS: r.DurationMS, Cost: r.Cost, CreatedAt: timeFromString(r.CreatedAt), UpdatedAt: timeFromString(r.UpdatedAt)}
}

func (m *StudioModel) ListWorkflows(ctx context.Context) ([]StudioWorkflow, error) {
	v := url.Values{"select": {"*"}, "order": {"updatedAt.desc"}}
	var rows []studioWorkflowRow
	if _, err := m.api.request(ctx, http.MethodGet, "StudioWorkflow", v, nil, "", &rows); err != nil {
		return nil, err
	}
	out := make([]StudioWorkflow, 0, len(rows))
	for _, r := range rows {
		out = append(out, workflowFromRow(r))
	}
	return out, nil
}
func (m *StudioModel) ListExecutions(ctx context.Context) ([]StudioExecution, error) {
	v := url.Values{"select": {"*"}, "order": {"startedAt.desc"}, "limit": {"100"}}
	var rows []studioExecutionRow
	if _, err := m.api.request(ctx, http.MethodGet, "StudioExecution", v, nil, "", &rows); err != nil {
		return nil, err
	}
	out := make([]StudioExecution, 0, len(rows))
	for _, r := range rows {
		out = append(out, executionFromRow(r))
	}
	return out, nil
}
func (m *StudioModel) Overview(ctx context.Context) ([]StudioWorkflow, []StudioExecution, error) {
	w, err := m.ListWorkflows(ctx)
	if err != nil {
		return nil, nil, err
	}
	e, err := m.ListExecutions(ctx)
	return w, e, err
}
func workflowBody(in StudioWorkflowInput) map[string]any {
	return map[string]any{"name": in.Name, "description": in.Description, "category": in.Category, "status": in.Status, "nodes": in.Nodes, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
}
func (m *StudioModel) CreateWorkflow(ctx context.Context, in StudioWorkflowInput) (*StudioWorkflow, error) {
	body := workflowBody(in)
	body["id"] = newID()
	var rows []studioWorkflowRow
	if _, err := m.api.request(ctx, http.MethodPost, "StudioWorkflow", url.Values{"select": {"*"}}, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	out := workflowFromRow(rows[0])
	return &out, nil
}
func (m *StudioModel) UpdateWorkflow(ctx context.Context, id string, in StudioWorkflowInput) (*StudioWorkflow, error) {
	var rows []studioWorkflowRow
	v := url.Values{"id": {"eq." + id}, "select": {"*"}}
	if _, err := m.api.request(ctx, http.MethodPatch, "StudioWorkflow", v, workflowBody(in), "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	out := workflowFromRow(rows[0])
	return &out, nil
}
func (m *StudioModel) FindExecution(ctx context.Context, id string) (*StudioExecution, error) {
	var rows []studioExecutionRow
	v := url.Values{"id": {"eq." + id}, "select": {"*"}, "limit": {"1"}}
	if _, err := m.api.request(ctx, http.MethodGet, "StudioExecution", v, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := executionFromRow(rows[0])
	return &out, nil
}
func (m *StudioModel) TransitionExecution(ctx context.Context, id, status string) (*StudioExecution, error) {
	var rows []studioExecutionRow
	v := url.Values{"id": {"eq." + id}, "select": {"*"}}
	body := map[string]any{"status": status, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	if _, err := m.api.request(ctx, http.MethodPatch, "StudioExecution", v, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	out := executionFromRow(rows[0])
	return &out, nil
}
