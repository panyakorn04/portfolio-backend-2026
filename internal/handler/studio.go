package handler

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

type studioStage struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	State  string `json:"state"`
}
type studioPublicWorkflow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Status      string   `json:"status"`
	Updated     string   `json:"updated"`
	Runs        int      `json:"runs"`
	Success     float64  `json:"success"`
	Nodes       []string `json:"nodes"`
}
type studioPublicExecution struct {
	ID         string  `json:"id"`
	Workflow   string  `json:"workflow"`
	Status     string  `json:"status"`
	Started    string  `json:"started"`
	Duration   string  `json:"duration"`
	DurationMS int     `json:"durationMs"`
	Cost       float64 `json:"cost"`
}
type studioOverview struct {
	Workflows  []studioPublicWorkflow  `json:"workflows"`
	Executions []studioPublicExecution `json:"executions"`
	Stages     []studioStage           `json:"stages"`
}
type studioWorkflowPayload struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Status      string   `json:"status"`
	Nodes       []string `json:"nodes"`
}

var workflowStatuses = map[string]bool{"active": true, "draft": true, "paused": true}
var executionTransitions = map[string]map[string]bool{
	"running": {"paused": true, "cancelled": true}, "paused": {"running": true, "cancelled": true},
	"failed": {"running": true, "cancelled": true}, "waiting": {"approved": true, "cancelled": true},
	"approved": {"running": true, "cancelled": true},
}

func publicStudioOverview(workflows []model.StudioWorkflow, executions []model.StudioExecution) studioOverview {
	publicWorkflows := make([]studioPublicWorkflow, 0, len(workflows))
	for _, item := range workflows {
		publicWorkflows = append(publicWorkflows, studioPublicWorkflow{ID: item.ID, Name: item.Name, Description: item.Description, Category: item.Category, Status: item.Status, Updated: item.UpdatedAt.UTC().Format(time.RFC3339), Runs: item.Runs, Success: item.Success, Nodes: item.Nodes})
	}
	publicExecutions := make([]studioPublicExecution, 0, len(executions))
	for _, item := range executions {
		minutes, seconds := item.DurationMS/60000, (item.DurationMS/1000)%60
		publicExecutions = append(publicExecutions, studioPublicExecution{ID: item.ID, Workflow: item.Workflow, Status: item.Status, Started: item.StartedAt.UTC().Format(time.RFC3339), Duration: fmt.Sprintf("%02d:%02d", minutes, seconds), DurationMS: item.DurationMS, Cost: item.Cost})
	}
	return studioOverview{Workflows: publicWorkflows, Executions: publicExecutions, Stages: seededStudioStages()}
}

func seededStudioOverview() studioOverview {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return publicStudioOverview([]model.StudioWorkflow{
		{ID: "wf-content", Name: "Content intelligence pipeline", Description: "Discover, analyze, generate, approve, and publish content.", Category: "Content", Status: "active", Runs: 1284, Success: 98.4, Nodes: []string{"Discover", "Analyze", "Generate", "Approval", "Publish"}, UpdatedAt: now},
		{ID: "wf-research", Name: "Competitive research brief", Description: "Turn multiple sources into a cited bilingual market brief.", Category: "Research", Status: "active", Runs: 486, Success: 96.8, Nodes: []string{"Search", "Extract", "Synthesize", "Review"}, UpdatedAt: now},
	}, []model.StudioExecution{{ID: "RUN-2841", WorkflowID: "wf-content", Workflow: "Content intelligence pipeline", Status: "running", StartedAt: now, DurationMS: 102000, Cost: .18}, {ID: "RUN-2840", WorkflowID: "wf-research", Workflow: "Competitive research brief", Status: "completed", StartedAt: now, DurationMS: 138000, Cost: .12}})
}
func seededStudioStages() []studioStage {
	return []studioStage{{"Discover", "Found 12 rights-allowed sources", "done"}, {"Analyze", "Ranked 8 candidate moments", "done"}, {"Generate", "Creating bilingual captions", "running"}, {"Approval", "Human review required", "pending"}, {"Publish", "Destination: Social queue", "pending"}}
}

// StudioOverviewHandler reads persisted Studio data, safely falling back to the portfolio seed when unavailable.
func StudioOverviewHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if svcCtx == nil || svcCtx.Studio == nil {
			response.Ok(w, http.StatusOK, seededStudioOverview())
			return
		}
		workflows, executions, err := svcCtx.Studio.Overview(r.Context())
		if err != nil {
			log.Printf("studio overview fallback: %v", err)
			response.Ok(w, http.StatusOK, seededStudioOverview())
			return
		}
		response.Ok(w, http.StatusOK, publicStudioOverview(workflows, executions))
	}
}

func validateStudioWorkflow(p studioWorkflowPayload) (model.StudioWorkflowInput, string) {
	p.Name = strings.TrimSpace(p.Name)
	p.Description = strings.TrimSpace(p.Description)
	p.Category = strings.TrimSpace(p.Category)
	p.Status = strings.ToLower(strings.TrimSpace(p.Status))
	if len([]rune(p.Name)) < 2 || len([]rune(p.Name)) > 120 {
		return model.StudioWorkflowInput{}, "Name must be between 2 and 120 characters."
	}
	if len([]rune(p.Description)) > 1000 {
		return model.StudioWorkflowInput{}, "Description must not exceed 1000 characters."
	}
	if len([]rune(p.Category)) < 2 || len([]rune(p.Category)) > 80 {
		return model.StudioWorkflowInput{}, "Category must be between 2 and 80 characters."
	}
	if !workflowStatuses[p.Status] {
		return model.StudioWorkflowInput{}, "Status must be active, draft, or paused."
	}
	if len(p.Nodes) < 1 || len(p.Nodes) > 30 {
		return model.StudioWorkflowInput{}, "Nodes must contain between 1 and 30 items."
	}
	for i := range p.Nodes {
		p.Nodes[i] = strings.TrimSpace(p.Nodes[i])
		if p.Nodes[i] == "" || len([]rune(p.Nodes[i])) > 80 {
			return model.StudioWorkflowInput{}, "Each node must be between 1 and 80 characters."
		}
	}
	return model.StudioWorkflowInput{Name: p.Name, Description: p.Description, Category: p.Category, Status: p.Status, Nodes: p.Nodes}, ""
}
func requireStudioDB(w http.ResponseWriter, s *svc.ServiceContext) bool {
	if !requireDatabase(w, s) {
		return false
	}
	if s.Studio == nil {
		response.Error(w, http.StatusServiceUnavailable, "Studio persistence is not configured.")
		return false
	}
	return true
}
func AdminListStudioWorkflowsHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, s); !ok {
			return
		}
		if !requireStudioDB(w, s) {
			return
		}
		items, err := s.Studio.ListWorkflows(r.Context())
		if err != nil {
			response.Error(w, 500, "Unable to load workflows.")
			return
		}
		response.Ok(w, 200, items)
	}
}
func AdminListStudioExecutionsHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, s); !ok {
			return
		}
		if !requireStudioDB(w, s) {
			return
		}
		items, err := s.Studio.ListExecutions(r.Context())
		if err != nil {
			response.Error(w, 500, "Unable to load executions.")
			return
		}
		response.Ok(w, 200, items)
	}
}
func AdminCreateStudioWorkflowHandler(s *svc.ServiceContext) http.HandlerFunc {
	return studioWorkflowMutation(s, true)
}
func AdminUpdateStudioWorkflowHandler(s *svc.ServiceContext) http.HandlerFunc {
	return studioWorkflowMutation(s, false)
}
func studioWorkflowMutation(s *svc.ServiceContext, create bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, s)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, s) {
			return
		}
		var p studioWorkflowPayload
		if !decodeJSON(w, r, &p) {
			return
		}
		in, msg := validateStudioWorkflow(p)
		if msg != "" {
			response.Error(w, 400, msg)
			return
		}
		var item *model.StudioWorkflow
		var err error
		if create {
			item, err = s.Studio.CreateWorkflow(r.Context(), in)
		} else {
			item, err = s.Studio.UpdateWorkflow(r.Context(), pathParam(r, "id"), in)
		}
		if errors.Is(err, model.ErrNotFound) {
			response.Error(w, 404, "Workflow was not found.")
			return
		}
		if err != nil {
			response.Error(w, 500, "Unable to save workflow.")
			return
		}
		code := 200
		if create {
			code = 201
		}
		response.Ok(w, code, item)
	}
}
func AdminStudioExecutionActionHandler(s *svc.ServiceContext, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, s)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, s) {
			return
		}
		target := map[string]string{"pause": "paused", "retry": "running", "cancel": "cancelled", "approve": "approved"}[action]
		current, err := s.Studio.FindExecution(r.Context(), pathParam(r, "id"))
		if err != nil {
			response.Error(w, 500, "Unable to load execution.")
			return
		}
		if current == nil {
			response.Error(w, 404, "Execution was not found.")
			return
		}
		if !executionTransitions[current.Status][target] {
			response.Error(w, 409, "Execution cannot transition from "+current.Status+" to "+target+".")
			return
		}
		item, err := s.Studio.TransitionExecution(r.Context(), current.ID, target)
		if err != nil {
			response.Error(w, 500, "Unable to update execution.")
			return
		}
		response.Ok(w, 200, item)
	}
}
