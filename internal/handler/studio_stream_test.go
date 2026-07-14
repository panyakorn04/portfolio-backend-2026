package handler

import (
	"bytes"
	"strings"
	"testing"

	"portfolio-backend/internal/model"
)

func TestWriteStudioSSEFramesSingleLineSafeJSON(t *testing.T) {
	var out bytes.Buffer
	id, err := writeStudioSSE(&out, "snapshot", map[string]string{"message": "safe\nvalue"})
	if err != nil || len(id) != 16 {
		t.Fatalf("id=%q err=%v", id, err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "id: "+id+"\nevent: snapshot\ndata: {") || !strings.HasSuffix(got, "}\n\n") || strings.Contains(got, "safe\nvalue") {
		t.Fatalf("invalid SSE frame %q", got)
	}
}

func TestPublicExecutionStagesNeverExposePersistedInputOrOutput(t *testing.T) {
	stages := []model.StudioExecutionStage{{
		NodeID: "request", Metadata: map[string]any{"private": true},
		Input:     []map[string]any{{"json": map[string]any{"password": "secret"}}},
		Output:    []map[string]any{{"json": map[string]any{"private": "value"}}},
		ErrorCode: "private_code", ErrorMessage: "private detail",
	}}
	public := publicExecutionStages(stages)
	if public[0].Input != nil || public[0].Output != nil || public[0].ErrorCode != "" || public[0].ErrorMessage != "" || len(public[0].Metadata) != 0 {
		t.Fatalf("public execution stage leaked private runtime data: %#v", public[0])
	}
	if stages[0].Input == nil || stages[0].Output == nil {
		t.Fatal("public projection mutated the persisted stage")
	}
}
