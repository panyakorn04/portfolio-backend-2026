package handler

import (
	"errors"
	"net/http"
	"strings"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

// AdminListUsersHandler -> GET /api/admin/users
func AdminListUsersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin"}) {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		items, err := svcCtx.Users.List(r.Context())
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load users.")
			return
		}
		response.Ok(w, http.StatusOK, map[string]any{"items": items})
	}
}

// AdminUpdateUserRoleHandler -> PATCH /api/admin/users/{id}
func AdminUpdateUserRoleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin"}) {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		var body struct {
			Role string `json:"role"`
		}
		_ = decodeBodyAllowEmpty(r, &body)
		role := strings.TrimSpace(body.Role)

		if role == "" || !auth.IsStaffRole(role) {
			response.Error(w, http.StatusBadRequest, "Role is invalid.",
				response.ErrorDetail{Field: "role", Message: "Use one of: " + strings.Join(auth.StaffRoles, ", ") + "."})
			return
		}

		id := pathParam(r, "id")
		if access.User != nil && access.User.ID == id && role != "admin" {
			response.Error(w, http.StatusBadRequest, "You cannot remove your own admin role.")
			return
		}

		user, err := svcCtx.Users.UpdateRole(r.Context(), id, role)
		if errors.Is(err, model.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "User not found.")
			return
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to update user.")
			return
		}
		response.Ok(w, http.StatusOK, user)
	}
}
