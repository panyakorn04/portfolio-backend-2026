package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/svc"
)

type StudioExecutionRunner struct {
	service                *svc.ServiceContext
	workerID               string
	cancel                 context.CancelFunc
	lastScheduleSlot       string
	lastScheduleOccurrence map[string]string
	wake                   chan struct{}
	wg                     sync.WaitGroup
}

func NewStudioExecutionRunner(service *svc.ServiceContext) *StudioExecutionRunner {
	return &StudioExecutionRunner{
		service: service, workerID: newStudioWorkerID(), wake: make(chan struct{}, 1),
		lastScheduleOccurrence: map[string]string{},
	}
}

func newStudioWorkerID() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("studio-worker-%d", time.Now().UnixNano())
	}
	return "studio-worker-" + hex.EncodeToString(value[:])
}

func (runner *StudioExecutionRunner) Start(parent context.Context) {
	if runner == nil || runner.service == nil || runner.service.Studio == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	runner.cancel = cancel
	runner.wg.Add(2)
	go runner.loop(ctx)
	go runner.scheduleLoop(ctx)
}

func (runner *StudioExecutionRunner) Close() {
	if runner == nil || runner.cancel == nil {
		return
	}
	runner.cancel()
	runner.wg.Wait()
}

func (runner *StudioExecutionRunner) Wake() {
	if runner == nil {
		return
	}
	select {
	case runner.wake <- struct{}{}:
	default:
	}
}

func (runner *StudioExecutionRunner) loop(ctx context.Context) {
	defer runner.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := runner.runAvailable(ctx); err != nil && ctx.Err() == nil {
			observability.Error(ctx, "studio.worker.run_failed", "Studio execution worker failed", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-runner.wake:
		}
	}
}

func (runner *StudioExecutionRunner) scheduleLoop(ctx context.Context) {
	defer runner.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if err := runner.enqueueDueSchedules(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			observability.Error(ctx, "studio.schedule.enqueue_failed", "Studio schedule enqueue failed", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (runner *StudioExecutionRunner) runAvailable(ctx context.Context) error {
	for ctx.Err() == nil {
		execution, err := runner.service.Studio.ClaimGraphExecution(ctx, runner.workerID)
		if err != nil {
			return err
		}
		if execution == nil {
			return nil
		}
		runner.execute(ctx, execution)
	}
	return ctx.Err()
}

func (runner *StudioExecutionRunner) execute(ctx context.Context, execution *model.StudioExecution) {
	fail := func(code, message string) {
		if _, err := runner.service.Studio.FinishGraphExecution(ctx, execution.ID, runner.workerID, "failed", code, message); err != nil {
			observability.Error(ctx, "studio.execution.finalization_failed", "Studio execution finalization failed", err)
		}
	}
	workflow, err := runner.service.Studio.FindWorkflow(ctx, execution.WorkflowID)
	if err != nil || workflow == nil || workflow.Definition == nil {
		fail("workflow_unavailable", "The workflow definition is unavailable.")
		return
	}
	if execution.WorkflowUpdatedAt == nil || !workflow.UpdatedAt.Equal(*execution.WorkflowUpdatedAt) {
		fail("workflow_changed", "The workflow changed after this execution was queued.")
		return
	}
	compileMode := studioGraphModeFull
	if execution.Mode == "through-target" {
		compileMode = studioGraphModeRootThroughTarget
	}
	compiled, err := compileStudioGraph(workflow.Definition, execution.TriggerNodeID, compileMode, execution.TargetNodeID)
	if err != nil {
		fail("invalid_graph", "The workflow graph is no longer executable.")
		return
	}
	triggerEnabled, _ := compiled.Nodes[0].Config["enabled"].(bool)
	if !triggerEnabled {
		fail("trigger_disabled", "The selected workflow trigger is disabled.")
		return
	}
	stages, err := runner.service.Studio.ListExecutionStages(ctx, execution.ID)
	if err != nil || len(stages) != len(compiled.Nodes) {
		fail("stage_mismatch", "The persisted execution stages do not match the workflow graph.")
		return
	}

	items := stages[0].Input
	for position, node := range compiled.Nodes {
		stage := stages[position]
		if stage.NodeID != node.ID || stage.NodeType != node.Type {
			fail("stage_mismatch", "The persisted execution stages do not match the workflow graph.")
			return
		}
		if stage.Status == "completed" {
			items = stage.Output
			continue
		}
		if execution.ErrorCode == "lease_recovered" && stage.Status == "running" {
			_, _ = runner.service.Studio.FinishExecutionStage(ctx, execution.ID, position, runner.workerID, "failed", nil, "dispatch_state_unknown", "The prior worker may have completed this external request.", "Stopped after worker recovery to prevent duplicate side effects")
			fail("dispatch_state_unknown", "Execution stopped after worker recovery because the prior external request state is unknown.")
			return
		}
		current, findErr := runner.service.Studio.FindExecution(ctx, execution.ID)
		if findErr != nil || current == nil {
			fail("execution_unavailable", "The execution is unavailable.")
			return
		}
		if current.Status == "cancellation_requested" || current.Status == "cancelled" {
			if stage.Status == "running" {
				_, _ = runner.service.Studio.FinishExecutionStage(ctx, execution.ID, position, runner.workerID, "cancelled", nil, "cancelled", "Execution cancelled.", "Execution cancelled")
			}
			_, _ = runner.service.Studio.FinishGraphExecution(ctx, execution.ID, runner.workerID, "cancelled", "cancelled", "Execution cancelled.")
			return
		}

		items = sanitizeStudioExecutionItems(items)
		started, startErr := runner.service.Studio.StartExecutionStage(ctx, execution.ID, position, runner.workerID, items)
		if startErr != nil || !started {
			fail("lease_lost", "The execution worker lost its lease.")
			return
		}

		var output []map[string]any
		var nodeErr *studioNodeExecutionError
		switch node.Kind {
		case "trigger":
			output = studioTriggerExecutionOutput(workflow.ID, node, items)
		case "action":
			if node.Type != "http-request" {
				nodeErr = &studioNodeExecutionError{Code: "unsupported_node", Message: "This workflow node is not executable."}
			} else {
				output, nodeErr = executeStudioHTTPRequestNode(ctx, runner.service, workflow.ID, node, items)
			}
		default:
			nodeErr = &studioNodeExecutionError{Code: "unsupported_node", Message: "This workflow node is not executable."}
		}
		if nodeErr != nil {
			_, _ = runner.service.Studio.FinishExecutionStage(ctx, execution.ID, position, runner.workerID, "failed", nil, nodeErr.Code, nodeErr.Message, nodeErr.Message)
			fail(nodeErr.Code, nodeErr.Message)
			return
		}
		current, findErr = runner.service.Studio.FindExecution(ctx, execution.ID)
		if findErr != nil || current == nil {
			fail("execution_unavailable", "The execution is unavailable.")
			return
		}
		if current.Status == "cancellation_requested" || current.Status == "cancelled" {
			_, _ = runner.service.Studio.FinishExecutionStage(ctx, execution.ID, position, runner.workerID, "cancelled", nil, "cancelled", "Execution cancelled.", "Execution cancelled")
			_, _ = runner.service.Studio.FinishGraphExecution(ctx, execution.ID, runner.workerID, "cancelled", "cancelled", "Execution cancelled.")
			return
		}
		output = sanitizeStudioExecutionItems(output)
		finished, finishErr := runner.service.Studio.FinishExecutionStage(ctx, execution.ID, position, runner.workerID, "completed", output, "", "", "Completed")
		if finishErr != nil || !finished {
			fail("lease_lost", "The execution worker lost its lease.")
			return
		}
		items = output
	}
	if _, err := runner.service.Studio.FinishGraphExecution(ctx, execution.ID, runner.workerID, "completed", "", ""); err != nil {
		observability.Error(ctx, "studio.execution.finalization_failed", "Studio execution finalization failed", err)
	}
}

func studioTriggerExecutionOutput(workflowID string, node model.StudioWorkflowNode, input []map[string]any) []map[string]any {
	if node.Type == "webhook" && len(input) > 0 {
		return input
	}
	payload := map[string]any{
		"trigger": node.Type, "mode": "execution", "workflowId": workflowID,
		"nodeId": node.ID, "executedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	if node.Type == "schedule" {
		payload["timezone"] = node.Config["timezone"]
		payload["scheduleMode"] = node.Config["mode"]
	}
	return []map[string]any{{"json": payload}}
}
