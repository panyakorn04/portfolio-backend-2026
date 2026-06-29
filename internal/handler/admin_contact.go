package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

var contactInquiryStatuses = []string{"new", "in_progress", "handled"}

func isContactInquiryStatus(value string) bool {
	for _, s := range contactInquiryStatuses {
		if s == value {
			return true
		}
	}
	return false
}

// AdminListInquiriesHandler -> GET /api/admin/contact-inquiries
func AdminListInquiriesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		q := r.URL.Query()
		limit := 20
		if v := q.Get("limit"); v != "" {
			limit, _ = strconv.Atoi(v)
		}
		page := 1
		if v := q.Get("page"); v != "" {
			page, _ = strconv.Atoi(v)
		}
		status := q.Get("status")
		if status == "" {
			status = "all"
		}

		result, err := svcCtx.Inquiries.List(r.Context(), model.ListInquiriesInput{
			Limit: limit, Page: page, Status: status, Query: strings.TrimSpace(q.Get("query")),
		})
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load inquiries.")
			return
		}
		response.Ok(w, http.StatusOK, result)
	}
}

// AdminGetInquiryHandler -> GET /api/admin/contact-inquiries/{id}
func AdminGetInquiryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		id := pathParam(r, "id")
		item, err := svcCtx.Inquiries.FindDetailByID(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load inquiry.")
			return
		}
		if item == nil {
			response.Error(w, http.StatusNotFound, "Contact inquiry was not found.")
			return
		}
		response.Ok(w, http.StatusOK, item)
	}
}

// AdminUpdateInquiryHandler -> PATCH /api/admin/contact-inquiries/{id}
func AdminUpdateInquiryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		var body struct {
			Status       string `json:"status"`
			InternalNote string `json:"internalNote"`
		}
		if err := decodeBodyAllowEmpty(r, &body); err != nil {
			response.Error(w, http.StatusBadRequest, "Request body must be valid JSON.")
			return
		}

		status := strings.TrimSpace(body.Status)
		internalNote := strings.TrimSpace(body.InternalNote)

		if status == "" || !isContactInquiryStatus(status) {
			response.Error(w, http.StatusBadRequest, "Status is invalid.",
				response.ErrorDetail{Field: "status", Message: "Use one of: " + strings.Join(contactInquiryStatuses, ", ") + "."})
			return
		}
		if len([]rune(internalNote)) > 2000 {
			response.Error(w, http.StatusBadRequest, "Internal note is too long.",
				response.ErrorDetail{Field: "internalNote", Message: "Keep the note under 2000 characters."})
			return
		}

		var notePtr *string
		if internalNote != "" {
			notePtr = &internalNote
		}

		actorType := "admin_token"
		actorLabel := "admin_token"
		if access.Via == auth.ViaSession {
			actorType = "admin_session"
			actorLabel = "admin_session"
			if access.User != nil {
				actorLabel = access.User.Email
			}
		}

		item, err := svcCtx.Inquiries.Update(r.Context(), pathParam(r, "id"), model.UpdateInquiryInput{
			Status: status, InternalNote: notePtr, ActorType: actorType, ActorLabel: actorLabel,
		})
		if errors.Is(err, model.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "Contact inquiry was not found.")
			return
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to update inquiry.")
			return
		}
		response.Ok(w, http.StatusOK, item)
	}
}
