package handler

import (
	"log"
	"net/http"
	"strings"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

type studioGraphExecutionPayload struct {
	WorkflowID    string           `json:"workflowId"`
	TriggerNodeID string           `json:"triggerNodeId"`
	TargetNodeID  string           `json:"targetNodeId"`
	Mode          string           `json:"mode"`
	SourceKey     string           `json:"sourceKey"`
	Input         []map[string]any `json:"input"`
}

func enqueueStudioWorkflowExecution(ctxRequest *http.Request, service *svc.ServiceContext, workflow *model.StudioWorkflow, payload studioGraphExecutionPayload, source, retryOf string) (*model.StudioExecution, string) {
	if workflow == nil || workflow.Definition == nil {
		return nil, "Save a structured workflow definition before executing it."
	}
	if _, message := validateWorkflowDefinition(workflow.Definition, true); message != "" {
		return nil, "Save a valid and complete workflow definition before executing it."
	}
	payload.TriggerNodeID = strings.TrimSpace(payload.TriggerNodeID)
	payload.TargetNodeID = strings.TrimSpace(payload.TargetNodeID)
	payload.Mode = strings.TrimSpace(payload.Mode)
	if payload.Mode == "" {
		payload.Mode = "full"
	}
	if payload.Mode != "full" && payload.Mode != "through-target" {
		return nil, "Execution mode must be full or through-target."
	}
	if payload.Mode == "through-target" && payload.TargetNodeID == "" {
		return nil, "A target node is required when executing previous nodes."
	}
	if payload.TriggerNodeID == "" {
		manualIDs := []string{}
		triggerIDs := []string{}
		for _, node := range workflow.Definition.Nodes {
			if node.Kind == "trigger" && isStudioRuntimeTriggerType(node.Type) {
				triggerIDs = append(triggerIDs, node.ID)
				if node.Type == "manual" {
					manualIDs = append(manualIDs, node.ID)
				}
			}
		}
		switch {
		case len(manualIDs) == 1:
			payload.TriggerNodeID = manualIDs[0]
		case len(triggerIDs) == 1:
			payload.TriggerNodeID = triggerIDs[0]
		default:
			return nil, "Select a trigger node for this execution."
		}
	}
	compileMode := studioGraphModeFull
	if payload.Mode == "through-target" {
		compileMode = studioGraphModeRootThroughTarget
	}
	compiled, err := compileStudioGraph(workflow.Definition, payload.TriggerNodeID, compileMode, payload.TargetNodeID)
	if err != nil {
		return nil, "The selected workflow path is not executable: " + err.Error()
	}
	triggerEnabled, _ := compiled.Nodes[0].Config["enabled"].(bool)
	if !triggerEnabled {
		return nil, "The selected workflow trigger is disabled."
	}
	path := make([]model.StudioExecutionPathNode, 0, len(compiled.Nodes))
	for _, node := range compiled.Nodes {
		path = append(path, model.StudioExecutionPathNode{ID: node.ID, Type: node.Type, Label: node.Label})
	}
	payload.SourceKey = strings.TrimSpace(payload.SourceKey)
	if len(payload.SourceKey) > 160 {
		return nil, "sourceKey must not exceed 160 characters."
	}
	sourceKey := payload.SourceKey
	if sourceKey != "" {
		sourceKey = workflow.ID + ":" + source + ":" + payload.TriggerNodeID + ":" + sourceKey
	}
	item, err := service.Studio.EnqueueGraphExecution(ctxRequest.Context(), model.StudioGraphExecutionInput{
		WorkflowID: workflow.ID, WorkflowName: workflow.Name, WorkflowUpdatedAt: workflow.UpdatedAt,
		TriggerNodeID: payload.TriggerNodeID, TargetNodeID: payload.TargetNodeID, Mode: payload.Mode,
		Source: source, SourceKey: sourceKey, RetryOfExecutionID: retryOf,
		InitialInput: sanitizeStudioExecutionItems(payload.Input), Path: path,
	})
	if err != nil {
		log.Printf("studio graph enqueue failed workflow=%q: %v", workflow.ID, err)
		return nil, "Unable to enqueue the workflow execution."
	}
	return item, ""
}

func AdminEnqueueStudioWorkflowHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		var payload studioGraphExecutionPayload
		if !decodeJSON(w, r, &payload) {
			return
		}
		workflowID := strings.TrimSpace(pathParam(r, "id"))
		if workflowID == "" {
			workflowID = strings.TrimSpace(payload.WorkflowID)
		}
		workflow, err := service.Studio.FindWorkflow(r.Context(), workflowID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load workflow.")
			return
		}
		if workflow == nil {
			response.Error(w, http.StatusNotFound, "Workflow was not found.")
			return
		}
		if workflow.Status == "paused" {
			response.Error(w, http.StatusConflict, "Paused workflows cannot be executed.")
			return
		}
		item, message := enqueueStudioWorkflowExecution(r, service, workflow, payload, "manual", "")
		if message != "" {
			response.Error(w, http.StatusConflict, message)
			return
		}
		if _, err := service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "execution.create", "execution", item.ID, "", item.Status)); err != nil {
			log.Printf("studio execution audit persistence failed: %v", err)
		}
		response.Ok(w, http.StatusAccepted, item)
	}
}

func AdminExecuteStudioPreviousNodesHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		var payload studioGraphExecutionPayload
		if !decodeOptionalJSON(w, r, &payload) {
			return
		}
		workflowID := strings.TrimSpace(pathParam(r, "id"))
		payload.TargetNodeID = strings.TrimSpace(pathParam(r, "nodeId"))
		payload.Mode = "through-target"
		workflow, err := service.Studio.FindWorkflow(r.Context(), workflowID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load workflow.")
			return
		}
		if workflow == nil {
			response.Error(w, http.StatusNotFound, "Workflow was not found.")
			return
		}
		if workflow.Status == "paused" {
			response.Error(w, http.StatusConflict, "Paused workflows cannot be executed.")
			return
		}
		item, message := enqueueStudioWorkflowExecution(r, service, workflow, payload, "node-test", "")
		if message != "" {
			response.Error(w, http.StatusConflict, message)
			return
		}
		if _, err := service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "node.execute-previous", "execution", item.ID, "", item.Status)); err != nil {
			log.Printf("studio execution audit persistence failed: %v", err)
		}
		response.Ok(w, http.StatusAccepted, item)
	}
}

func AdminGetStudioExecutionHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		detail, err := service.Studio.ExecutionDetail(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load execution.")
			return
		}
		if detail == nil {
			response.Error(w, http.StatusNotFound, "Execution was not found.")
			return
		}
		response.Ok(w, http.StatusOK, detail)
	}
}

func AdminCancelStudioGraphExecutionHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		item, err := service.Studio.CancelGraphExecution(r.Context(), id)
		if err != nil {
			if err == model.ErrNotFound {
				response.Error(w, http.StatusConflict, "Only queued or running executions can be cancelled.")
				return
			}
			response.Error(w, http.StatusInternalServerError, "Unable to cancel execution.")
			return
		}
		_, _ = service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "execution.cancel", "execution", id, "", item.Status))
		response.Ok(w, http.StatusOK, item)
	}
}

func AdminRetryStudioGraphExecutionHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		original, err := service.Studio.ExecutionDetail(r.Context(), id)
		if err != nil || original == nil {
			response.Error(w, http.StatusNotFound, "Execution was not found.")
			return
		}
		if original.Execution.Status != "failed" && original.Execution.Status != "cancelled" {
			response.Error(w, http.StatusConflict, "Only failed or cancelled executions can be retried.")
			return
		}
		workflow, err := service.Studio.FindWorkflow(r.Context(), original.Execution.WorkflowID)
		if err != nil || workflow == nil {
			response.Error(w, http.StatusConflict, "The workflow is unavailable.")
			return
		}
		initial := []map[string]any{}
		if len(original.Stages) > 0 {
			initial = original.Stages[0].Input
		}
		payload := studioGraphExecutionPayload{TriggerNodeID: original.Execution.TriggerNodeID, TargetNodeID: original.Execution.TargetNodeID, Mode: original.Execution.Mode, SourceKey: "retry-" + id, Input: initial}
		item, message := enqueueStudioWorkflowExecution(r, service, workflow, payload, "retry", id)
		if message != "" {
			response.Error(w, http.StatusConflict, message)
			return
		}
		_, _ = service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "execution.retry", "execution", item.ID, id, item.Status))
		response.Ok(w, http.StatusAccepted, item)
	}
}

func AdminDeleteStudioWorkflowHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin"}) || !requireStudioDB(w, service) || !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		workflow, err := service.Studio.FindWorkflow(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load workflow.")
			return
		}
		if workflow == nil {
			response.Error(w, http.StatusNotFound, "Workflow was not found.")
			return
		}
		hasActiveExecutions, err := service.Studio.HasActiveExecutions(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to inspect workflow executions.")
			return
		}
		if hasActiveExecutions {
			response.Error(w, http.StatusConflict, "Cancel the running workflow execution before deleting this workflow.")
			return
		}
		deleted, err := service.Studio.DeleteWorkflowIfIdle(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to delete workflow.")
			return
		}
		if !deleted {
			response.Error(w, http.StatusConflict, "Workflow changed or started running before it could be deleted.")
			return
		}
		_, _ = service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "workflow.delete", "workflow", id, workflow.Status, "deleted"))
		response.Ok(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
	}
}

func AdminUnsupportedStudioExecutionActionHandler(service *svc.ServiceContext, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		response.Error(w, http.StatusConflict, "Execution "+action+" is not supported by the graph runtime.")
	}
}

func decodeOptionalJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	return decodeJSON(w, r, target)
}
