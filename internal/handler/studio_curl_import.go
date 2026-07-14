package handler

import (
	"net/http"

	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

type studioCurlImportPayload struct {
	Command string `json:"command"`
}

func AdminImportStudioCurlHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxStudioCurlCommandBytes+1024)
		var payload studioCurlImportPayload
		if !decodeJSON(w, r, &payload) {
			return
		}
		result, err := parseStudioCurlCommand(payload.Command)
		if err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		response.Ok(w, http.StatusOK, result)
	}
}
