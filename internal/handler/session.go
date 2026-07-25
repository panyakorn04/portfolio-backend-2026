package handler

import (
	"net/http"
	"strings"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

func setSessionCookie(w http.ResponseWriter, svcCtx *svc.ServiceContext, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   shouldSecureCookie(svcCtx),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	})
}

// SessionStatusHandler -> GET /api/admin/session
func SessionStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		response.Ok(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"via":           access.Via,
			"user":          access.User,
		})
	}
}

// SessionLoginHandler -> POST /api/admin/session
func SessionLoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceLoginRateLimit(w, r, svcCtx) {
			return
		}
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		_ = decodeBodyAllowEmpty(r, &body)

		email := strings.ToLower(strings.TrimSpace(body.Email))
		password := strings.TrimSpace(body.Password)

		if email == "" || password == "" {
			field := "email"
			if email != "" {
				field = "password"
			}
			response.Error(w, http.StatusBadRequest, "email and password are required.",
				response.ErrorDetail{Field: field, Message: "Provide valid login credentials."})
			return
		}

		if !svcCtx.HasDatabse {
			response.Error(w, http.StatusServiceUnavailable, "Unable to sign in right now.")
			return
		}

		user, err := svcCtx.Users.FindByEmail(r.Context(), email)
		if err != nil {
			response.Error(w, http.StatusServiceUnavailable, "Unable to sign in right now.")
			return
		}

		if user == nil || !auth.IsStaffRole(user.Role) {
			response.Error(w, http.StatusUnauthorized, "Admin credentials are invalid.")
			return
		}

		if !auth.VerifyPassword(password, user.PasswordHash) {
			response.Error(w, http.StatusUnauthorized, "Admin credentials are invalid.")
			return
		}

		rawToken, err := auth.CreateRawSessionToken()
		if err != nil {
			response.Error(w, http.StatusServiceUnavailable, "Unable to sign in right now.")
			return
		}

		if _, err := svcCtx.Sessions.Create(r.Context(), user.ID,
			auth.HashSessionToken(rawToken), auth.SessionExpiry()); err != nil {
			response.Error(w, http.StatusServiceUnavailable, "Unable to sign in right now.")
			return
		}

		setSessionCookie(w, svcCtx, rawToken, auth.SessionMaxAgeSeconds)

		response.Ok(w, http.StatusOK, map[string]any{
			"authenticated": true,
			"via":           "session",
			"user": map[string]any{
				"id": user.ID, "email": user.Email, "name": user.Name, "role": user.Role,
			},
		})
	}
}

// SessionLogoutHandler -> DELETE /api/admin/session
func SessionLogoutHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rawToken := auth.GetCookieValue(r, auth.SessionCookieName)
		if rawToken != "" && svcCtx.HasDatabse {
			if err := svcCtx.Sessions.DeleteByTokenHash(r.Context(), auth.HashSessionToken(rawToken)); err != nil {
				observability.Error(r.Context(), "admin_session.logout_revoke_failed", "Admin session logout revocation failed", err)
				response.Error(w, http.StatusServiceUnavailable, "Unable to sign out right now.")
				return
			}
		}
		setSessionCookie(w, svcCtx, "", -1)
		response.Ok(w, http.StatusOK, map[string]any{"authenticated": false})
	}
}
