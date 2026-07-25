package handler

import (
	"net/http"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

// AdminListSessionsHandler -> GET /api/admin/sessions
func AdminListSessionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, auth.StaffRoles) {
			return
		}
		if access.User == nil {
			response.Error(w, http.StatusBadRequest, "Session listing requires a user-backed session.")
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		currentRaw := auth.GetCookieValue(r, auth.SessionCookieName)
		var currentID string
		if currentRaw != "" {
			if session, err := svcCtx.Sessions.FindByTokenHash(r.Context(), auth.HashSessionToken(currentRaw)); err == nil && session != nil {
				currentID = session.ID
			}
		}

		sessions, err := svcCtx.Sessions.ListForUser(r.Context(), access.User.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load sessions.")
			return
		}

		items := make([]map[string]any, 0, len(sessions))
		for _, s := range sessions {
			items = append(items, map[string]any{
				"id": s.ID, "expiresAt": s.ExpiresAt, "createdAt": s.CreatedAt,
				"lastSeenAt": s.LastSeenAt, "isCurrent": s.ID == currentID,
			})
		}
		response.Ok(w, http.StatusOK, map[string]any{"items": items})
	}
}

// AdminLogoutEverywhereHandler -> DELETE /api/admin/sessions
func AdminLogoutEverywhereHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, auth.StaffRoles) {
			return
		}
		if access.User == nil {
			response.Error(w, http.StatusBadRequest, "Logout everywhere requires a user-backed session.")
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		if err := svcCtx.Sessions.DeleteAllForUser(r.Context(), access.User.ID); err != nil {
			observability.Error(r.Context(), "admin_session.logout_everywhere_revoke_failed", "Admin logout-everywhere revocation failed", err)
			response.Error(w, http.StatusInternalServerError, "Unable to log out all sessions.")
			return
		}
		setSessionCookie(w, svcCtx, "", -1)

		response.Ok(w, http.StatusOK, map[string]any{
			"authenticated": false, "loggedOutEverywhere": true,
		})
	}
}

// AdminRevokeSessionHandler -> DELETE /api/admin/sessions/{id}
func AdminRevokeSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, auth.StaffRoles) {
			return
		}
		if access.User == nil {
			response.Error(w, http.StatusBadRequest, "Session revoke requires a user-backed session.")
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		id := pathParam(r, "id")
		currentRaw := auth.GetCookieValue(r, auth.SessionCookieName)
		var currentID string
		if currentRaw != "" {
			if session, err := svcCtx.Sessions.FindByTokenHash(r.Context(), auth.HashSessionToken(currentRaw)); err == nil && session != nil {
				currentID = session.ID
			}
		}

		deleted, err := svcCtx.Sessions.DeleteByID(r.Context(), id, access.User.ID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to revoke session.")
			return
		}
		if deleted == 0 {
			response.Error(w, http.StatusNotFound, "Session not found.")
			return
		}

		isCurrent := currentID == id
		if isCurrent {
			setSessionCookie(w, svcCtx, "", -1)
		}
		response.Ok(w, http.StatusOK, map[string]any{"revoked": true, "isCurrent": isCurrent})
	}
}
