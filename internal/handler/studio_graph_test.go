package handler

import (
	"strings"
	"testing"

	"portfolio-backend/internal/model"
)

func studioGraphDefinition() *model.StudioWorkflowDefinition {
	return &model.StudioWorkflowDefinition{
		Version: 1,
		Nodes: []model.StudioWorkflowNode{
			{ID: "request-2", Type: "http-request", Kind: "action", Label: "Second"},
			{ID: "manual", Type: "manual", Kind: "trigger", Label: "Manual"},
			{ID: "request-1", Type: "http-request", Kind: "action", Label: "First"},
			{ID: "schedule", Type: "schedule", Kind: "trigger", Label: "Schedule"},
			{ID: "webhook-1", Type: "webhook", Kind: "trigger", Label: "Webhook"},
		},
		Edges: []model.StudioWorkflowEdge{
			{ID: "second-to-third", Source: "request-1", Target: "request-2"},
			{ID: "manual-to-first", Source: "manual", Target: "request-1"},
			{ID: "schedule-to-first", Source: "schedule", Target: "request-1"},
			{ID: "webhook-to-first", Source: "webhook-1", Target: "request-1"},
		},
	}
}

func TestCompileStudioGraphSelectsRootAndPreservesPathOrder(t *testing.T) {
	definition := studioGraphDefinition()
	compiled, err := compileStudioGraph(definition, "manual", studioGraphModeFull, "")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if compiled.Root.ID != "manual" {
		t.Fatalf("root=%q", compiled.Root.ID)
	}
	if got := studioGraphNodeIDs(compiled.Nodes); strings.Join(got, ",") != "manual,request-1,request-2" {
		t.Fatalf("nodes=%v", got)
	}
	if len(compiled.Edges) != 2 || compiled.Edges[0].ID != "manual-to-first" || compiled.Edges[1].ID != "second-to-third" {
		t.Fatalf("edges=%#v", compiled.Edges)
	}

	// The compiler is pure: compiling does not reorder or rewrite the definition.
	if definition.Nodes[0].ID != "request-2" || definition.Edges[0].ID != "second-to-third" {
		t.Fatalf("definition was mutated: %#v", definition)
	}
}

func TestCompileStudioGraphRootThroughTarget(t *testing.T) {
	compiled, err := compileStudioGraph(studioGraphDefinition(), "schedule", studioGraphModeRootThroughTarget, "request-1")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := strings.Join(studioGraphNodeIDs(compiled.Nodes), ","); got != "schedule,request-1" {
		t.Fatalf("nodes=%s", got)
	}
	if len(compiled.Edges) != 1 || compiled.Edges[0].ID != "schedule-to-first" {
		t.Fatalf("edges=%#v", compiled.Edges)
	}
}

func TestCompileStudioGraphSupportsEveryExecutableTrigger(t *testing.T) {
	for _, runtimeType := range []string{"manual", "schedule", "webhook"} {
		t.Run(runtimeType, func(t *testing.T) {
			compiled, err := compileStudioGraph(studioGraphDefinition(), runtimeType, studioGraphModeFull, "")
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if compiled.Root.Type != runtimeType {
				t.Fatalf("root type=%q", compiled.Root.Type)
			}
		})
	}
}

func TestCompileStudioGraphCanSelectTriggerByNodeID(t *testing.T) {
	compiled, err := compileStudioGraph(studioGraphDefinition(), "webhook-1", studioGraphModeFull, "")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if compiled.Root.ID != "webhook-1" || compiled.Root.Type != "webhook" {
		t.Fatalf("root=%#v", compiled.Root)
	}
}

func TestCompileStudioGraphRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name        string
		definition  func() *model.StudioWorkflowDefinition
		runtimeType string
		mode        studioGraphCompileMode
		target      string
		message     string
	}{
		{
			name: "ambiguous root",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Nodes = append(d.Nodes, model.StudioWorkflowNode{ID: "manual-2", Type: "manual", Kind: "trigger"})
				d.Edges = append(d.Edges, model.StudioWorkflowEdge{ID: "manual-2-to-first", Source: "manual-2", Target: "request-1"})
				return d
			},
			runtimeType: "manual", mode: studioGraphModeFull, message: "ambiguous",
		},
		{
			name: "cycle",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Edges = append(d.Edges, model.StudioWorkflowEdge{ID: "cycle", Source: "request-2", Target: "request-1"})
				return d
			},
			runtimeType: "manual", mode: studioGraphModeFull, message: "cycle",
		},
		{
			name: "disconnected target",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Nodes = append(d.Nodes, model.StudioWorkflowNode{ID: "orphan", Type: "http-request", Kind: "action"})
				return d
			},
			runtimeType: "manual", mode: studioGraphModeRootThroughTarget, target: "orphan", message: "not reachable",
		},
		{
			name: "ambiguous path",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Nodes = append(d.Nodes, model.StudioWorkflowNode{ID: "request-branch", Type: "http-request", Kind: "action"})
				d.Edges = append(d.Edges, model.StudioWorkflowEdge{ID: "manual-to-branch", Source: "manual", Target: "request-branch"})
				return d
			},
			runtimeType: "manual", mode: studioGraphModeFull, message: "ambiguous execution path",
		},
		{
			name: "tampered dangling edge",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Edges[0].Target = "missing"
				return d
			},
			runtimeType: "manual", mode: studioGraphModeFull, message: "unknown node",
		},
		{
			name: "tampered kind",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Nodes[1].Kind = "action"
				return d
			},
			runtimeType: "manual", mode: studioGraphModeFull, message: "type and kind",
		},
		{
			name:        "unsupported runtime trigger",
			definition:  studioGraphDefinition,
			runtimeType: "email", mode: studioGraphModeFull, message: "unsupported runtime type",
		},
		{
			name: "unsupported runtime action",
			definition: func() *model.StudioWorkflowDefinition {
				d := studioGraphDefinition()
				d.Nodes[2].Type = "transform"
				return d
			},
			runtimeType: "manual", mode: studioGraphModeFull, message: "unsupported runtime type",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := compileStudioGraph(test.definition(), test.runtimeType, test.mode, test.target)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.message) {
				t.Fatalf("error=%v, want message containing %q", err, test.message)
			}
		})
	}
}

func studioGraphNodeIDs(nodes []model.StudioWorkflowNode) []string {
	ids := make([]string, len(nodes))
	for i := range nodes {
		ids[i] = nodes[i].ID
	}
	return ids
}
