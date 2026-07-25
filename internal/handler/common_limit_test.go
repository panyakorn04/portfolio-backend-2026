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

func TestDecodeJSONWithLimitRejectsOversizedTrailingWhitespace(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/contact",
		strings.NewReader(`{"name":"ok"}`+strings.Repeat(" ", 100)),
	)
	var payload map[string]any

	if decodeJSONWithLimit(recorder, request, &payload, 32) {
		t.Fatal("decodeJSONWithLimit() = true, want false")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusRequestEntityTooLarge, recorder.Body.String())
	}
}

func TestDecodeJSONRejectsMultipleValues(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/contact", strings.NewReader(`{"name":"ok"}{"extra":true}`))
	var payload map[string]any

	if decodeJSON(recorder, request, &payload) {
		t.Fatal("decodeJSON() = true, want false")
	}
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
