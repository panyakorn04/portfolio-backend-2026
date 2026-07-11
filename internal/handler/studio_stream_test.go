package handler

import (
	"bytes"
	"strings"
	"testing"
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
