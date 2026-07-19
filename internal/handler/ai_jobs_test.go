package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/svc"
)

func TestJobsContactFollowUpFailsClosedWhenProcessorIsUnavailable(t *testing.T) {
	service := &svc.ServiceContext{Config: config.Config{InternalApiToken: "internal-test-token"}}
	request := httptest.NewRequest(http.MethodPost, "/api/jobs/contact-follow-up", strings.NewReader(`{"inquiryId":"inquiry-1"}`))
	request.Header.Set("Authorization", "Bearer internal-test-token")
	recorder := httptest.NewRecorder()

	JobsContactFollowUpHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotImplemented, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "inquiry-1") {
		t.Fatal("response echoed inquiry data")
	}
}

func TestJobsContactFollowUpStillRequiresInternalAuthentication(t *testing.T) {
	service := &svc.ServiceContext{Config: config.Config{InternalApiToken: "internal-test-token"}}
	request := httptest.NewRequest(http.MethodPost, "/api/jobs/contact-follow-up", nil)
	recorder := httptest.NewRecorder()

	JobsContactFollowUpHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
