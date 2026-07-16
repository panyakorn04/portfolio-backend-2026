package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONWithLimitRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/admin/articles",
		strings.NewReader(`{"content":"too large"}`),
	)
	var payload map[string]any

	if decodeJSONWithLimit(recorder, request, &payload, 8) {
		t.Fatal("decodeJSONWithLimit() = true, want false")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
}
