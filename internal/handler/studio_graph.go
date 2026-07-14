package handler

import (
	"errors"
	"fmt"

	"portfolio-backend/internal/model"
)

// studioGraphCompileMode controls how far the compiler follows the selected
// trigger's path. Full mode compiles through the terminal node. Root-through-
// target mode compiles an inclusive prefix ending at TargetNodeID.
type studioGraphCompileMode string

const (
	studioGraphModeFull              studioGraphCompileMode = "full"
	studioGraphModeRootThroughTarget studioGraphCompileMode = "root-through-target"
)

// studioCompiledGraph is an execution projection of a persisted workflow
// definition. Nodes and edges are ordered as an executable path, independent
// of their order in the persisted slices.
type studioCompiledGraph struct {
	Root  model.StudioWorkflowNode
	Nodes []model.StudioWorkflowNode
	Edges []model.StudioWorkflowEdge
}

// compileStudioGraph compiles the path belonging to exactly one runtime
// trigger type. It is deliberately pure: it performs no I/O and never mutates
// definition.
func compileStudioGraph(definition *model.StudioWorkflowDefinition, triggerSelector string, mode studioGraphCompileMode, targetNodeID string) (studioCompiledGraph, error) {
	if definition == nil {
		return studioCompiledGraph{}, errors.New("studio graph definition is required")
	}
	if mode != studioGraphModeFull && mode != studioGraphModeRootThroughTarget {
		return studioCompiledGraph{}, fmt.Errorf("unsupported studio graph compile mode %q", mode)
	}
	if mode == studioGraphModeRootThroughTarget && targetNodeID == "" {
		return studioCompiledGraph{}, errors.New("root-through-target mode requires a target node")
	}

	nodesByID := make(map[string]model.StudioWorkflowNode, len(definition.Nodes))
	for _, node := range definition.Nodes {
		if node.ID == "" {
			return studioCompiledGraph{}, errors.New("tampered studio graph contains an empty node ID")
		}
		if _, exists := nodesByID[node.ID]; exists {
			return studioCompiledGraph{}, fmt.Errorf("tampered studio graph contains duplicate node ID %q", node.ID)
		}
		if expectedKind, known := studioNodeKinds[node.Type]; known && node.Kind != expectedKind {
			return studioCompiledGraph{}, fmt.Errorf("tampered studio graph node %q has invalid type and kind", node.ID)
		}
		nodesByID[node.ID] = node
	}
	if len(nodesByID) == 0 {
		return studioCompiledGraph{}, errors.New("studio graph contains no nodes")
	}
	if mode == studioGraphModeRootThroughTarget {
		if _, exists := nodesByID[targetNodeID]; !exists {
			return studioCompiledGraph{}, fmt.Errorf("target node %q does not exist", targetNodeID)
		}
	}

	outgoing := make(map[string][]model.StudioWorkflowEdge, len(nodesByID))
	indegree := make(map[string]int, len(nodesByID))
	edgeIDs := make(map[string]struct{}, len(definition.Edges))
	edgePairs := make(map[string]struct{}, len(definition.Edges))
	for _, edge := range definition.Edges {
		if edge.ID == "" {
			return studioCompiledGraph{}, errors.New("tampered studio graph contains an empty edge ID")
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			return studioCompiledGraph{}, fmt.Errorf("tampered studio graph contains duplicate edge ID %q", edge.ID)
		}
		edgeIDs[edge.ID] = struct{}{}
		if _, exists := nodesByID[edge.Source]; !exists {
			return studioCompiledGraph{}, fmt.Errorf("tampered edge %q references unknown node %q", edge.ID, edge.Source)
		}
		target, exists := nodesByID[edge.Target]
		if !exists {
			return studioCompiledGraph{}, fmt.Errorf("tampered edge %q references unknown node %q", edge.ID, edge.Target)
		}
		if edge.Source == edge.Target {
			return studioCompiledGraph{}, fmt.Errorf("tampered edge %q is a self-edge", edge.ID)
		}
		if target.Kind == "trigger" {
			return studioCompiledGraph{}, fmt.Errorf("tampered edge %q points into trigger node %q", edge.ID, edge.Target)
		}
		pair := edge.Source + "\x00" + edge.Target
		if _, exists := edgePairs[pair]; exists {
			return studioCompiledGraph{}, fmt.Errorf("tampered studio graph contains duplicate edge %q to %q", edge.Source, edge.Target)
		}
		edgePairs[pair] = struct{}{}
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
		indegree[edge.Target]++
	}

	if err := detectStudioGraphCycle(nodesByID, outgoing); err != nil {
		return studioCompiledGraph{}, err
	}

	roots := make([]model.StudioWorkflowNode, 0, 1)
	if isStudioRuntimeTriggerType(triggerSelector) {
		for _, node := range definition.Nodes {
			if node.Type == triggerSelector && node.Kind == "trigger" {
				roots = append(roots, node)
			}
		}
	} else if selected, exists := nodesByID[triggerSelector]; exists {
		if selected.Kind != "trigger" || !isStudioRuntimeTriggerType(selected.Type) {
			return studioCompiledGraph{}, fmt.Errorf("unsupported runtime type %q at node %q", selected.Type, selected.ID)
		}
		roots = append(roots, selected)
	} else {
		return studioCompiledGraph{}, fmt.Errorf("unsupported runtime type or trigger node %q", triggerSelector)
	}
	if len(roots) == 0 {
		return studioCompiledGraph{}, fmt.Errorf("studio graph has no %q trigger root", triggerSelector)
	}
	if len(roots) > 1 {
		return studioCompiledGraph{}, fmt.Errorf("ambiguous %q trigger root: found %d", triggerSelector, len(roots))
	}
	if indegree[roots[0].ID] != 0 {
		return studioCompiledGraph{}, fmt.Errorf("tampered trigger node %q is not a root", roots[0].ID)
	}

	root := roots[0]
	runtimeType := root.Type
	compiled := studioCompiledGraph{
		Root:  root,
		Nodes: make([]model.StudioWorkflowNode, 0, len(nodesByID)),
		Edges: make([]model.StudioWorkflowEdge, 0, len(definition.Edges)),
	}
	current := root
	for {
		if current.Type != runtimeType && current.Type != "http-request" {
			return studioCompiledGraph{}, fmt.Errorf("unsupported runtime type %q at node %q", current.Type, current.ID)
		}
		compiled.Nodes = append(compiled.Nodes, current)
		if mode == studioGraphModeRootThroughTarget && current.ID == targetNodeID {
			return compiled, nil
		}
		nextEdges := outgoing[current.ID]
		if len(nextEdges) == 0 {
			if mode == studioGraphModeRootThroughTarget {
				return studioCompiledGraph{}, fmt.Errorf("target node %q is not reachable from trigger root %q", targetNodeID, root.ID)
			}
			return compiled, nil
		}
		if len(nextEdges) > 1 {
			return studioCompiledGraph{}, fmt.Errorf("ambiguous execution path from node %q", current.ID)
		}
		edge := nextEdges[0]
		compiled.Edges = append(compiled.Edges, edge)
		current = nodesByID[edge.Target]
	}
}

func isStudioRuntimeTriggerType(nodeType string) bool {
	return nodeType == "manual" || nodeType == "schedule" || nodeType == "webhook"
}

func detectStudioGraphCycle(nodes map[string]model.StudioWorkflowNode, outgoing map[string][]model.StudioWorkflowEdge) error {
	const (
		unvisited uint8 = iota
		visiting
		visited
	)
	state := make(map[string]uint8, len(nodes))
	var visit func(string) error
	visit = func(nodeID string) error {
		switch state[nodeID] {
		case visiting:
			return fmt.Errorf("studio graph contains a cycle at node %q", nodeID)
		case visited:
			return nil
		}
		state[nodeID] = visiting
		for _, edge := range outgoing[nodeID] {
			if err := visit(edge.Target); err != nil {
				return err
			}
		}
		state[nodeID] = visited
		return nil
	}
	for nodeID := range nodes {
		if err := visit(nodeID); err != nil {
			return err
		}
	}
	return nil
}
