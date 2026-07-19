package handler

import (
	"net/http"
	"strings"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/logic"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

// AiContactSummaryHandler -> POST /api/ai/contact-summary
func AiContactSummaryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		var body struct {
			InquiryID string `json:"inquiryId"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}

		inquiryID := strings.TrimSpace(body.InquiryID)
		if inquiryID == "" {
			response.Error(w, http.StatusBadRequest, "inquiryId is required.")
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		inquiry, err := svcCtx.Inquiries.FindByID(r.Context(), inquiryID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load inquiry.")
			return
		}
		if inquiry == nil {
			response.Error(w, http.StatusNotFound, "Inquiry not found.")
			return
		}

		summary := logic.GenerateContactSummary(svcCtx, inquiry)
		response.Ok(w, http.StatusOK, summary)
	}
}

// JobsContactFollowUpHandler -> POST /api/jobs/contact-follow-up
func JobsContactFollowUpHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, accessErr := auth.RequireInternal(svcCtx, r); accessErr != nil {
			response.Error(w, accessErr.Status, accessErr.Message)
			return
		}

		response.Error(w, http.StatusNotImplemented, "Contact follow-up processing is not configured.")
	}
}
