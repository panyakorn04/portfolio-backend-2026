package handler

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const maxStudioCredentialPayloadBytes = 32 << 10

type studioCredentialPayload struct {
	Name string            `json:"name"`
	Type string            `json:"type"`
	Data map[string]string `json:"data"`
}

func requireStudioCredentialCipher(w http.ResponseWriter, service *svc.ServiceContext) bool {
	if service.StudioCredentialCipher == nil {
		response.Error(w, http.StatusServiceUnavailable, "Studio credential encryption is not configured.")
		return false
	}
	return true
}

func validateStudioCredentialPayload(payload studioCredentialPayload) error {
	payload.Name = strings.TrimSpace(payload.Name)
	if len(payload.Name) < 2 || len(payload.Name) > 120 {
		return errors.New("Credential name must contain between 2 and 120 characters")
	}
	expected := map[string][]string{
		"bearer": {"token"},
		"basic":  {"username", "password"},
		"header": {"name", "value"},
		"query":  {"name", "value"},
	}
	keys, ok := expected[payload.Type]
	if !ok || len(payload.Data) != len(keys) {
		return errors.New("Credential type or fields are invalid")
	}
	for _, key := range keys {
		value := payload.Data[key]
		if value == "" || len(value) > maxStudioHTTPHeaderValueBytes || strings.ContainsAny(value, "\x00\r\n") {
			return errors.New("Credential fields are invalid")
		}
	}
	switch payload.Type {
	case "header":
		if validateStudioCredentialHeaders(map[string]string{payload.Data["name"]: payload.Data["value"]}) != nil {
			return errors.New("Credential header is invalid")
		}
	case "query":
		name := strings.TrimSpace(payload.Data["name"])
		if name == "" || len(name) > maxStudioHTTPQueryNameBytes {
			return errors.New("Credential query parameter is invalid")
		}
	}
	return nil
}

func studioCredentialCipherScope(id, credentialType string) string {
	return id + ":" + credentialType + ":v1"
}

func AdminListStudioCredentialsHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, service); !ok {
			return
		}
		if !requireStudioDB(w, service) || !requireStudioCredentialCipher(w, service) {
			return
		}
		items, err := service.Studio.ListCredentials(r.Context())
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load credentials.")
			return
		}
		response.Ok(w, http.StatusOK, items)
	}
}

func AdminCreateStudioCredentialHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !requireStudioCredentialCipher(w, service) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxStudioCredentialPayloadBytes)
		var payload studioCredentialPayload
		if !decodeJSON(w, r, &payload) {
			return
		}
		payload.Name = strings.TrimSpace(payload.Name)
		if err := validateStudioCredentialPayload(payload); err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		credentialID := model.NewStudioCredentialID()
		encrypted, err := service.StudioCredentialCipher.EncryptFor(studioCredentialCipherScope(credentialID, payload.Type), payload.Data)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to encrypt credential.")
			return
		}
		item, err := service.Studio.CreateCredential(r.Context(), model.StudioCredentialInput{ID: credentialID, Name: payload.Name, Type: payload.Type, EncryptedData: encrypted})
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to create credential.")
			return
		}
		if _, err := service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "credential.create", "credential", item.ID, "", "created")); err != nil {
			log.Printf("studio credential audit persistence failed: %v", err)
			response.Error(w, http.StatusInternalServerError, "Credential created but its audit record could not be saved.")
			return
		}
		response.Ok(w, http.StatusCreated, item.StudioCredential)
	}
}

func AdminUpdateStudioCredentialHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !requireStudioCredentialCipher(w, service) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		if id == "" || len(id) > 128 {
			response.Error(w, http.StatusBadRequest, "Credential id is required.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxStudioCredentialPayloadBytes)
		var payload studioCredentialPayload
		if !decodeJSON(w, r, &payload) {
			return
		}
		payload.Name = strings.TrimSpace(payload.Name)
		if err := validateStudioCredentialPayload(payload); err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
		encrypted, err := service.StudioCredentialCipher.EncryptFor(studioCredentialCipherScope(id, payload.Type), payload.Data)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to encrypt credential.")
			return
		}
		item, err := service.Studio.UpdateCredential(r.Context(), id, model.StudioCredentialInput{Name: payload.Name, Type: payload.Type, EncryptedData: encrypted})
		if errors.Is(err, model.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "Credential was not found.")
			return
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to update credential.")
			return
		}
		if _, err := service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "credential.update", "credential", item.ID, "", "updated")); err != nil {
			log.Printf("studio credential audit persistence failed: %v", err)
			response.Error(w, http.StatusInternalServerError, "Credential updated but its audit record could not be saved.")
			return
		}
		response.Ok(w, http.StatusOK, item.StudioCredential)
	}
}

func AdminDeleteStudioCredentialHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !requireStudioCredentialCipher(w, service) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		if id == "" || len(id) > 128 {
			response.Error(w, http.StatusBadRequest, "Credential id is required.")
			return
		}
		item, err := service.Studio.FindCredential(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load credential.")
			return
		}
		if item == nil {
			response.Error(w, http.StatusNotFound, "Credential was not found.")
			return
		}
		workflows, err := service.Studio.ListWorkflows(r.Context())
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to check credential references.")
			return
		}
		for _, workflow := range workflows {
			if workflow.Definition == nil {
				continue
			}
			for _, node := range workflow.Definition.Nodes {
				if credentialID, _ := node.Config["credentialId"].(string); credentialID == id {
					response.Error(w, http.StatusConflict, "Credential is still referenced by a workflow.")
					return
				}
			}
		}
		if err := service.Studio.DeleteCredential(r.Context(), id); err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to delete credential.")
			return
		}
		if _, err := service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "credential.delete", "credential", id, "", "deleted")); err != nil {
			log.Printf("studio credential audit persistence failed: %v", err)
			response.Error(w, http.StatusInternalServerError, "Credential deleted but its audit record could not be saved.")
			return
		}
		response.Ok(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func AdminTestStudioCredentialHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) || !requireStudioCredentialCipher(w, service) {
			return
		}
		if !enforceStudioMutationRateLimit(w, r, service, access) {
			return
		}
		id := strings.TrimSpace(pathParam(r, "id"))
		item, err := service.Studio.FindCredential(r.Context(), id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load credential.")
			return
		}
		if item == nil {
			response.Error(w, http.StatusNotFound, "Credential was not found.")
			return
		}
		data, err := service.StudioCredentialCipher.DecryptFor(studioCredentialCipherScope(item.ID, item.Type), item.EncryptedData)
		if err != nil || validateStudioCredentialPayload(studioCredentialPayload{Name: item.Name, Type: item.Type, Data: data}) != nil {
			response.Error(w, http.StatusConflict, "Credential could not be decrypted or validated.")
			return
		}
		if _, err := service.Studio.CreateAudit(r.Context(), studioAuditInput(r, service, access, "credential.test", "credential", id, "", "valid")); err != nil {
			log.Printf("studio credential audit persistence failed: %v", err)
			response.Error(w, http.StatusInternalServerError, "Credential tested but its audit record could not be saved.")
			return
		}
		response.Ok(w, http.StatusOK, map[string]any{"valid": true, "type": item.Type})
	}
}
