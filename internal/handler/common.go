package handler

import (
	"encoding/json"
	"net/http"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

// pathParam reads a named path variable populated by go-zero's router.
func pathParam(r *http.Request, name string) string {
	return pathvar.Vars(r)[name]
}

// decodeJSON parses the request body into v. Returns false (and writes a 400) on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		response.Error(w, http.StatusBadRequest, "Request body must be valid JSON.")
		return false
	}
	return true
}

// decodeBodyAllowEmpty decodes the body but tolerates a missing/invalid body,
// matching the original `request.json().catch(() => null)` behavior.
func decodeBodyAllowEmpty(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

// requireDatabase writes a 503 and returns false when Supabase REST is not configured.
func requireDatabase(w http.ResponseWriter, svcCtx *svc.ServiceContext) bool {
	if !svcCtx.HasDatabse {
		response.Error(w, http.StatusServiceUnavailable, "Service is not configured yet.",
			response.ErrorDetail{Field: "NEXT_PUBLIC_SUPABASE_URL", Message: "Add Supabase REST URL and key configuration."})
		return false
	}
	return true
}

// requireAdmin runs admin auth and writes the access error response on failure.
func requireAdmin(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext) (*auth.AccessContext, bool) {
	access, accessErr := auth.RequireAdmin(r.Context(), svcCtx, r)
	if accessErr != nil {
		response.Error(w, accessErr.Status, accessErr.Message)
		return nil, false
	}
	return access, true
}

func assertRole(w http.ResponseWriter, access *auth.AccessContext, allowed []string) bool {
	if err := auth.AssertStaffRole(access, allowed); err != nil {
		response.Error(w, err.Status, err.Message)
		return false
	}
	return true
}

// shouldSecureCookie decides Secure based on site URL / environment.
func shouldSecureCookie(svcCtx *svc.ServiceContext) bool {
	if svcCtx.Config.SiteURL != "" {
		return len(svcCtx.Config.SiteURL) >= 8 && svcCtx.Config.SiteURL[:8] == "https://"
	}
	return svcCtx.Config.Mode == "pro"
}
