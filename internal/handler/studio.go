package handler

import (
	"net/http"

	"portfolio-backend/internal/response"
)

type studioWorkflow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Status      string   `json:"status"`
	Runs        int      `json:"runs"`
	Success     float64  `json:"success"`
	Updated     string   `json:"updated"`
	Nodes       []string `json:"nodes"`
}

type studioExecution struct {
	ID         string  `json:"id"`
	Workflow   string  `json:"workflow"`
	Status     string  `json:"status"`
	Started    string  `json:"started"`
	Duration   string  `json:"duration"`
	DurationMS int     `json:"durationMs"`
	Cost       float64 `json:"cost"`
}

type studioStage struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	State  string `json:"state"`
}

type studioOverview struct {
	Workflows  []studioWorkflow  `json:"workflows"`
	Executions []studioExecution `json:"executions"`
	Stages     []studioStage     `json:"stages"`
}

// StudioOverviewHandler returns a public, read-only portfolio demonstration model.
// Mutating workflow operations remain unavailable until authenticated persistence is added.
func StudioOverviewHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		response.Ok(w, http.StatusOK, studioOverview{
			Workflows: []studioWorkflow{
				{ID: "wf-content", Name: "Content intelligence pipeline", Description: "Discover, analyze, generate, approve, and publish content.", Category: "Content", Status: "active", Runs: 1284, Success: 98.4, Updated: "2 min ago", Nodes: []string{"Discover", "Analyze", "Generate", "Approval", "Publish"}},
				{ID: "wf-research", Name: "Competitive research brief", Description: "Turn multiple sources into a cited bilingual market brief.", Category: "Research", Status: "active", Runs: 486, Success: 96.8, Updated: "18 min ago", Nodes: []string{"Search", "Extract", "Synthesize", "Review"}},
				{ID: "wf-meeting", Name: "Meeting action center", Description: "Summarize meetings, identify owners, and sync action items.", Category: "Operations", Status: "draft", Runs: 72, Success: 94.1, Updated: "Yesterday", Nodes: []string{"Transcript", "Summarize", "Assign", "Sync"}},
			},
			Executions: []studioExecution{
				{ID: "RUN-2841", Workflow: "Content intelligence pipeline", Status: "running", Started: "Now", Duration: "01:42", DurationMS: 102000, Cost: 0.18},
				{ID: "RUN-2840", Workflow: "Competitive research brief", Status: "completed", Started: "12 min ago", Duration: "02:18", DurationMS: 138000, Cost: 0.12},
				{ID: "RUN-2839", Workflow: "Meeting action center", Status: "waiting", Started: "26 min ago", Duration: "00:48", DurationMS: 48000, Cost: 0.06},
				{ID: "RUN-2838", Workflow: "Content intelligence pipeline", Status: "failed", Started: "1 hr ago", Duration: "00:34", DurationMS: 34000, Cost: 0.03},
				{ID: "RUN-2837", Workflow: "Content intelligence pipeline", Status: "completed", Started: "2 hrs ago", Duration: "02:06", DurationMS: 126000, Cost: 0.16},
			},
			Stages: []studioStage{
				{Name: "Discover", Detail: "Found 12 rights-allowed sources", State: "done"},
				{Name: "Analyze", Detail: "Ranked 8 candidate moments", State: "done"},
				{Name: "Generate", Detail: "Creating bilingual captions", State: "running"},
				{Name: "Approval", Detail: "Human review required", State: "pending"},
				{Name: "Publish", Detail: "Destination: Social queue", State: "pending"},
			},
		})
	}
}
