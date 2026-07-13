package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"portfolio-backend/internal/auth"
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
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Category    string                          `json:"category"`
	Status      string                          `json:"status"`
	Nodes       []string                        `json:"nodes"`
	Definition  *model.StudioWorkflowDefinition `json:"definition"`
}

type studioExecutionPayload struct {
	WorkflowID string `json:"workflowId"`
}

type studioTriggerOutput struct {
	NodeID     string                      `json:"nodeId"`
	NodeType   string                      `json:"nodeType"`
	ExecutedAt string                      `json:"executedAt"`
	Output     []map[string]map[string]any `json:"output"`
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
	if p.Definition != nil {
		labels, message := validateWorkflowDefinition(p.Definition, p.Status == "active")
		if message != "" {
			return model.StudioWorkflowInput{}, message
		}
		if len(p.Nodes) > 0 {
			for i := range p.Nodes {
				p.Nodes[i] = strings.TrimSpace(p.Nodes[i])
				if i >= len(labels) || p.Nodes[i] != labels[i] {
					return model.StudioWorkflowInput{}, "Nodes must match the workflow definition."
				}
			}
			if len(p.Nodes) != len(labels) {
				return model.StudioWorkflowInput{}, "Nodes must match the workflow definition."
			}
		}
		p.Nodes = labels
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
	return model.StudioWorkflowInput{Name: p.Name, Description: p.Description, Category: p.Category, Status: p.Status, Nodes: p.Nodes, Definition: p.Definition}, ""
}

var studioNodeKinds = map[string]string{
	"schedule": "trigger", "webhook": "trigger", "manual": "trigger",
	"search": "action", "analyze": "action", "generate": "action", "extract": "action", "transform": "action",
	"review": "logic", "approve": "logic", "condition": "logic", "route": "logic",
	"publish": "output", "notify": "output", "sync": "output", "export": "output",
}

func validateWorkflowDefinition(definition *model.StudioWorkflowDefinition, requireComplete bool) ([]string, string) {
	if definition.Version != 1 || len(definition.Nodes) < 1 || len(definition.Nodes) > 30 {
		return nil, "Workflow definition must use version 1 and contain between 1 and 30 nodes."
	}
	encoded, err := json.Marshal(definition)
	if err != nil || len(encoded) > 128*1024 {
		return nil, "Workflow definition is invalid or too large."
	}
	ids := map[string]bool{}
	triggerCount := 0
	for _, node := range definition.Nodes {
		expectedKind, ok := studioNodeKinds[node.Type]
		if !ok || expectedKind != node.Kind {
			return nil, "Workflow node type and kind are invalid."
		}
		if node.ID == "" || len(node.ID) > 128 || ids[node.ID] {
			return nil, "Workflow node IDs must be unique and no longer than 128 characters."
		}
		ids[node.ID] = true
		node.Label = strings.TrimSpace(node.Label)
		if node.Label == "" || len([]rune(node.Label)) > 80 || math.IsNaN(node.Position.X) || math.IsInf(node.Position.X, 0) || math.IsNaN(node.Position.Y) || math.IsInf(node.Position.Y, 0) {
			return nil, "Workflow node metadata is invalid."
		}
		if node.Kind == "trigger" {
			triggerCount++
		}

		if message := validateNodeConfig(node, requireComplete); message != "" {
			return nil, message
		}
	}
	if triggerCount == 0 {
		return nil, "Workflow definition must contain at least one trigger."
	}
	sorted := append([]model.StudioWorkflowNode(nil), definition.Nodes...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Position.X == sorted[j].Position.X {
			return sorted[i].Position.Y < sorted[j].Position.Y
		}
		return sorted[i].Position.X < sorted[j].Position.X
	})
	triggers := make([]model.StudioWorkflowNode, 0, triggerCount)
	steps := make([]model.StudioWorkflowNode, 0, len(sorted)-triggerCount)
	for _, node := range sorted {
		if node.Kind == "trigger" {
			triggers = append(triggers, node)
		} else {
			steps = append(steps, node)
		}
	}
	if len(steps) > 0 {
		for _, trigger := range triggers {
			if trigger.Position.X >= steps[0].Position.X {
				return nil, "All trigger nodes must be positioned before workflow steps."
			}
		}
	}
	expectedPairs := map[string]bool{}
	if len(steps) > 0 {
		for _, trigger := range triggers {
			expectedPairs[trigger.ID+"\x00"+steps[0].ID] = true
		}
		for index := 0; index < len(steps)-1; index++ {
			expectedPairs[steps[index].ID+"\x00"+steps[index+1].ID] = true
		}
	}
	if len(definition.Edges) != len(expectedPairs) {
		return nil, "Workflow edges must connect every trigger to the shared step chain."
	}
	edgeIDs := map[string]bool{}
	edgePairs := map[string]bool{}
	for _, edge := range definition.Edges {
		pair := edge.Source + "\x00" + edge.Target
		if edge.ID == "" || edgeIDs[edge.ID] || edgePairs[pair] || edge.Source == edge.Target || !ids[edge.Source] || !ids[edge.Target] || !expectedPairs[pair] {
			return nil, "Workflow edges must be unique and follow the trigger-to-step graph."
		}
		edgeIDs[edge.ID] = true
		edgePairs[pair] = true
	}
	labels := make([]string, len(sorted))
	for i := range sorted {
		labels[i] = strings.TrimSpace(sorted[i].Label)
	}
	return labels, ""
}

var studioNodeConfigKeys = map[string]map[string]bool{
	"schedule": {"enabled": true, "mode": true, "timezone": true, "intervalMinutes": true, "time": true, "daysOfWeek": true, "cronExpression": true, "misfirePolicy": true},
	"manual":   {"enabled": true},
	"webhook":  {"enabled": true, "method": true, "authMode": true, "responseMode": true},
}

func validateNodeConfig(node model.StudioWorkflowNode, requireComplete bool) string {
	allowed := studioNodeConfigKeys[node.Type]
	if allowed == nil {
		if len(node.Config) > 0 {
			return "This workflow node type does not accept parameters in definition version 1."
		}
		return ""
	}
	for key := range node.Config {
		if !allowed[key] {
			return "Workflow node config contains an unsupported parameter."
		}
	}
	if node.Type == "schedule" && requireComplete {
		return validateScheduleConfig(node.Config)
	}
	if enabled, exists := node.Config["enabled"]; exists {
		if _, ok := enabled.(bool); !ok {
			return "Node enabled must be true or false."
		}
	} else if requireComplete {
		return "Node enabled must be true or false."
	}
	if node.Type == "webhook" && requireComplete {
		method, methodOK := node.Config["method"].(string)
		authMode, authOK := node.Config["authMode"].(string)
		responseMode, responseOK := node.Config["responseMode"].(string)
		if !methodOK || (method != "GET" && method != "POST") || !authOK || authMode != "none" || !responseOK || responseMode != "immediate" {
			return "Webhook configuration is invalid for definition version 1."
		}
	}
	return ""
}

func configNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	default:
		return 0, false
	}
}

func validWeekdays(value any) bool {
	seen := map[int]bool{}
	add := func(day int) bool {
		if day < 0 || day > 6 || seen[day] {
			return false
		}
		seen[day] = true
		return true
	}
	switch days := value.(type) {
	case []any:
		for _, raw := range days {
			number, ok := configNumber(raw)
			if !ok || number != math.Trunc(number) || !add(int(number)) {
				return false
			}
		}
	case []int:
		for _, day := range days {
			if !add(day) {
				return false
			}
		}
	default:
		return false
	}
	return len(seen) > 0
}

func validCronNumber(raw string, minValue, maxValue int) bool {
	value, err := strconv.Atoi(raw)
	return err == nil && value >= minValue && value <= maxValue
}

func validCronField(field string, minValue, maxValue int) bool {
	if field == "" {
		return false
	}
	for _, item := range strings.Split(field, ",") {
		parts := strings.Split(item, "/")
		if len(parts) > 2 {
			return false
		}
		base := parts[0]
		if len(parts) == 2 {
			step, err := strconv.Atoi(parts[1])
			if err != nil || step < 1 || step > maxValue-minValue+1 {
				return false
			}
		}
		if base == "*" {
			continue
		}
		rangeParts := strings.Split(base, "-")
		if len(rangeParts) == 1 {
			if !validCronNumber(rangeParts[0], minValue, maxValue) {
				return false
			}
			continue
		}
		if len(rangeParts) != 2 || !validCronNumber(rangeParts[0], minValue, maxValue) || !validCronNumber(rangeParts[1], minValue, maxValue) {
			return false
		}
		start, _ := strconv.Atoi(rangeParts[0])
		end, _ := strconv.Atoi(rangeParts[1])
		if start > end {
			return false
		}
	}
	return true
}

func validCronExpression(expression string) bool {
	fields := strings.Fields(expression)
	if len(fields) != 5 {
		return false
	}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for index, field := range fields {
		if !validCronField(field, ranges[index][0], ranges[index][1]) {
			return false
		}
	}
	return true
}

func validateScheduleConfig(config map[string]any) string {
	if _, ok := config["enabled"].(bool); !ok {
		return "Schedule enabled must be true or false."
	}
	mode, _ := config["mode"].(string)
	timezone, _ := config["timezone"].(string)
	misfire, _ := config["misfirePolicy"].(string)
	if _, err := time.LoadLocation(timezone); err != nil {
		return "Schedule timezone must be a valid IANA timezone."
	}
	if misfire != "skip" && misfire != "run-once" {
		return "Schedule misfire policy must be skip or run-once."
	}
	switch mode {
	case "interval":
		minutes, ok := configNumber(config["intervalMinutes"])
		if !ok || minutes != math.Trunc(minutes) || minutes < 1 || minutes > 43200 {
			return "Schedule interval must be between 1 and 43,200 minutes."
		}
	case "daily":
		clock, _ := config["time"].(string)
		if _, err := time.Parse("15:04", clock); err != nil {
			return "Schedule time must use HH:mm in 24-hour format."
		}
	case "weekly":
		clock, _ := config["time"].(string)
		if _, err := time.Parse("15:04", clock); err != nil {
			return "Schedule time must use HH:mm in 24-hour format."
		}
		if !validWeekdays(config["daysOfWeek"]) {
			return "Weekly schedules must select at least one valid day."
		}
	case "cron":
		expression, _ := config["cronExpression"].(string)
		if !validCronExpression(expression) {
			return "Schedule cron expression must be a valid 5-field numeric cron expression."
		}
	default:
		return "Schedule mode must be interval, daily, weekly, or cron."
	}
	return ""
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
func AdminGetStudioWorkflowHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, s); !ok {
			return
		}
		if !requireStudioDB(w, s) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		if id == "" || len(id) > 128 {
			response.Error(w, http.StatusBadRequest, "Workflow id is required.")
			return
		}
		item, err := s.Studio.FindWorkflow(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load workflow.")
			return
		}
		if item == nil {
			response.Error(w, http.StatusNotFound, "Workflow was not found.")
			return
		}
		response.Ok(w, http.StatusOK, item)
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
func AdminExecuteStudioTriggerHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, s)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, s) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, s, access) {
			return
		}
		workflowID := strings.TrimSpace(pathParam(r, "id"))
		nodeID := strings.TrimSpace(pathParam(r, "nodeId"))
		if workflowID == "" || nodeID == "" || len(workflowID) > 128 || len(nodeID) > 128 {
			response.Error(w, http.StatusBadRequest, "Workflow and node IDs are required.")
			return
		}
		workflow, err := s.Studio.FindWorkflow(r.Context(), workflowID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load workflow.")
			return
		}
		if workflow == nil {
			response.Error(w, http.StatusNotFound, "Workflow was not found.")
			return
		}
		if workflow.Status == "paused" {
			response.Error(w, http.StatusConflict, "Paused workflows cannot test triggers.")
			return
		}
		if workflow.Definition == nil {
			response.Error(w, http.StatusConflict, "Save a structured workflow definition before executing a node.")
			return
		}
		if _, message := validateWorkflowDefinition(workflow.Definition, true); message != "" {
			response.Error(w, http.StatusConflict, "Save a valid and complete workflow definition before executing a trigger.")
			return
		}
		var triggerNode *model.StudioWorkflowNode
		for index := range workflow.Definition.Nodes {
			node := &workflow.Definition.Nodes[index]
			if node.ID == nodeID {
				triggerNode = node
				break
			}
		}
		if triggerNode == nil {
			response.Error(w, http.StatusNotFound, "Workflow node was not found.")
			return
		}
		if triggerNode.Kind != "trigger" || !map[string]bool{"manual": true, "schedule": true, "webhook": true}[triggerNode.Type] {
			response.Error(w, http.StatusBadRequest, "Only workflow trigger nodes support Execute step.")
			return
		}
		if enabled, ok := triggerNode.Config["enabled"].(bool); !ok || !enabled {
			response.Error(w, http.StatusConflict, "Enable the workflow trigger before executing it.")
			return
		}
		executedAt := time.Now().UTC().Format(time.RFC3339Nano)
		jsonOutput := map[string]any{
			"trigger": triggerNode.Type, "mode": "test", "workflowId": workflow.ID,
			"nodeId": nodeID, "executedAt": executedAt,
		}
		if triggerNode.Type == "schedule" {
			jsonOutput["timezone"] = triggerNode.Config["timezone"]
			jsonOutput["scheduleMode"] = triggerNode.Config["mode"]
		}
		if triggerNode.Type == "webhook" {
			jsonOutput["method"] = triggerNode.Config["method"]
			jsonOutput["headers"] = map[string]any{}
			jsonOutput["query"] = map[string]any{}
			jsonOutput["body"] = map[string]any{}
		}
		item := studioTriggerOutput{
			NodeID: nodeID, NodeType: triggerNode.Type, ExecutedAt: executedAt,
			Output: []map[string]map[string]any{{"json": jsonOutput}},
		}
		if _, err := s.Studio.CreateAudit(r.Context(), studioAuditInput(r, s, access, "node.execute", "workflow-node", nodeID, "", "completed")); err != nil {
			log.Printf("studio trigger audit persistence failed: %v", err)
			response.Error(w, http.StatusInternalServerError, "Node executed but its audit record could not be saved.")
			return
		}
		response.Ok(w, http.StatusOK, item)
	}
}

func AdminCreateStudioExecutionHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, s)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, s) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, s, access) {
			return
		}
		var p studioExecutionPayload
		if !decodeJSON(w, r, &p) {
			return
		}
		p.WorkflowID = strings.TrimSpace(p.WorkflowID)
		if p.WorkflowID == "" || len([]rune(p.WorkflowID)) > 128 {
			response.Error(w, 400, "workflowId is required and must not exceed 128 characters.")
			return
		}
		workflow, err := s.Studio.FindWorkflow(r.Context(), p.WorkflowID)
		if err != nil {
			response.Error(w, 500, "Unable to load workflow.")
			return
		}
		if workflow == nil {
			response.Error(w, 404, "Workflow was not found.")
			return
		}
		if workflow.Status != "active" {
			response.Error(w, 409, "Only active workflows can be run.")
			return
		}
		item, err := s.Studio.CreateExecution(r.Context(), workflow.ID, workflow.Name, workflow.Nodes)
		if err != nil {
			response.Error(w, 500, "Unable to create execution.")
			return
		}
		if _, err := s.Studio.CreateAudit(r.Context(), studioAuditInput(r, s, access, "execution.create", "execution", item.ID, "", item.Status)); err != nil {
			log.Printf("studio audit persistence failed: %v", err)
			response.Error(w, 500, "Execution created but its audit record could not be saved.")
			return
		}
		response.Ok(w, 201, item)
	}
}
func AdminListStudioAuditsHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, s); !ok {
			return
		}
		if !requireStudioDB(w, s) {
			return
		}
		items, err := s.Studio.ListAudits(r.Context(), 50)
		if err != nil {
			response.Error(w, 500, "Unable to load Studio audit log.")
			return
		}
		response.Ok(w, 200, items)
	}
}
func studioAuditInput(r *http.Request, s *svc.ServiceContext, access *auth.AccessContext, action, resourceType, resourceID, from, to string) model.StudioAuditInput {
	actorType, actorLabel, actorID := string(access.Via), "admin bearer", ""
	if access.User != nil {
		actorID, actorLabel = access.User.ID, access.User.Email
	}
	ua := r.UserAgent()
	if len(ua) > 256 {
		ua = ua[:256]
	}
	return model.StudioAuditInput{ActorType: actorType, ActorID: actorID, ActorLabel: actorLabel, Action: action, ResourceType: resourceType, ResourceID: resourceID, FromStatus: from, ToStatus: to, Metadata: map[string]any{"ip": clientIP(r, s.Config.TrustProxy), "method": r.Method, "path": r.URL.Path, "userAgent": ua}}
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
		if !enforceStudioMutationRateLimit(w, r, s, access) {
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
		fromStatus := ""
		if create {
			item, err = s.Studio.CreateWorkflow(r.Context(), in)
		} else {
			current, findErr := s.Studio.FindWorkflow(r.Context(), pathParam(r, "id"))
			if findErr != nil {
				response.Error(w, 500, "Unable to load workflow.")
				return
			}
			if current == nil {
				response.Error(w, 404, "Workflow was not found.")
				return
			}
			fromStatus = current.Status
			item, err = s.Studio.UpdateWorkflow(r.Context(), current.ID, in)
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
		action := "workflow.update"
		if create {
			action = "workflow.create"
		}
		if _, err := s.Studio.CreateAudit(r.Context(), studioAuditInput(r, s, access, action, "workflow", item.ID, fromStatus, item.Status)); err != nil {
			log.Printf("studio audit persistence failed: %v", err)
			response.Error(w, 500, "Workflow changed but its audit record could not be saved.")
			return
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
		if !enforceStudioMutationRateLimit(w, r, s, access) {
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
		if _, err := s.Studio.CreateAudit(r.Context(), studioAuditInput(r, s, access, "execution."+action, "execution", item.ID, current.Status, item.Status)); err != nil {
			log.Printf("studio audit persistence failed: %v", err)
			response.Error(w, 500, "Execution changed but its audit record could not be saved.")
			return
		}
		response.Ok(w, 200, item)
	}
}
