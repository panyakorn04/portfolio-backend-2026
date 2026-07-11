package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

var studioSSEPollInterval = 2 * time.Second
var studioSSEHeartbeatInterval = 15 * time.Second
var studioSSEMaxLifetime = 5 * time.Minute

type studioExecutionSnapshot struct {
	Execution studioPublicExecution        `json:"execution"`
	Stages    []model.StudioExecutionStage `json:"stages"`
}

func publicExecutionStages(stages []model.StudioExecutionStage) []model.StudioExecutionStage {
	out := make([]model.StudioExecutionStage, len(stages))
	for i, stage := range stages {
		stage.Metadata = map[string]any{}
		out[i] = stage
	}
	return out
}

func loadStudioExecutionSnapshot(ctx context.Context, studio *model.StudioModel, id string) (*studioExecutionSnapshot, error) {
	execution, err := studio.FindExecution(ctx, id)
	if err != nil {
		return nil, err
	}
	if execution == nil {
		return nil, model.ErrNotFound
	}
	stages, err := studio.ListExecutionStages(ctx, id)
	if err != nil {
		return nil, err
	}
	public := publicStudioOverview(nil, []model.StudioExecution{*execution}).Executions[0]
	return &studioExecutionSnapshot{Execution: public, Stages: publicExecutionStages(stages)}, nil
}

func StudioExecutionStagesHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.Studio == nil {
			response.Error(w, http.StatusServiceUnavailable, "Studio persistence is not configured.")
			return
		}
		stages, err := s.Studio.ListExecutionStages(r.Context(), pathParam(r, "id"))
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load execution stages.")
			return
		}
		response.Ok(w, http.StatusOK, publicExecutionStages(stages))
	}
}

func writeStudioSSE(w io.Writer, event string, value any) (string, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	id := hex.EncodeToString(sum[:8])
	_, err = fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", id, event, b)
	return id, err
}

func StudioExecutionEventsHandler(s *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s == nil || s.Studio == nil {
			response.Error(w, http.StatusServiceUnavailable, "Studio persistence is not configured.")
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			response.Error(w, http.StatusInternalServerError, "Streaming is unsupported.")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		id := pathParam(r, "id")
		poll := time.NewTicker(studioSSEPollInterval)
		defer poll.Stop()
		heartbeat := time.NewTicker(studioSSEHeartbeatInterval)
		defer heartbeat.Stop()
		deadline := time.NewTimer(studioSSEMaxLifetime)
		defer deadline.Stop()
		last := ""
		send := func() bool {
			snapshot, err := loadStudioExecutionSnapshot(r.Context(), s.Studio, id)
			if err != nil {
				_, _ = writeStudioSSE(w, "error", map[string]string{"message": "Execution updates are temporarily unavailable."})
				flusher.Flush()
				return false
			}
			b, _ := json.Marshal(snapshot)
			sum := sha256.Sum256(b)
			next := hex.EncodeToString(sum[:8])
			if next != last {
				_, _ = writeStudioSSE(w, "snapshot", snapshot)
				last = next
				flusher.Flush()
			}
			return true
		}
		if !send() {
			return
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case <-deadline.C:
				_, _ = writeStudioSSE(w, "reconnect", map[string]string{"reason": "stream_lifetime"})
				flusher.Flush()
				return
			case <-heartbeat.C:
				_, _ = fmt.Fprint(w, ": heartbeat\n\n")
				flusher.Flush()
			case <-poll.C:
				if !send() {
					return
				}
			}
		}
	}
}
