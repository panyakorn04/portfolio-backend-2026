package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"portfolio-backend/internal/svc"
)

func TestArticlesHandlerRejectsLimitAboveMaximum(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/articles?lang=en&limit=101", nil)
	recorder := httptest.NewRecorder()

	ArticlesHandler(&svc.ServiceContext{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}
