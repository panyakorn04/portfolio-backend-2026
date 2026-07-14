package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/security"
	"portfolio-backend/internal/svc"
)

func TestAdminCreateStudioCredentialEncryptsAndRedactsSecret(t *testing.T) {
	t.Parallel()

	var persisted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/rest/v1/StudioCredential" {
			if err := json.NewDecoder(r.Body).Decode(&persisted); err != nil {
				t.Fatal(err)
			}
			row := map[string]any{
				"id": persisted["id"], "name": persisted["name"], "type": persisted["type"],
				"encryptedData": persisted["encryptedData"],
				"createdAt":     "2026-01-01T00:00:00Z", "updatedAt": "2026-01-01T00:00:00Z",
			}
			_ = json.NewEncoder(w).Encode([]any{row})
			return
		}
		if r.URL.Path == "/rest/v1/StudioAuditLog" {
			_, _ = w.Write([]byte(`[{"id":"audit-1","actorType":"bearer","actorLabel":"api-token","action":"credential.create","resourceType":"credential","resourceId":"cred-1","metadata":{},"createdAt":"2026-01-01T00:00:00Z"}]`))
			return
		}
		t.Fatalf("unexpected persistence path %s", r.URL.Path)
	}))
	defer server.Close()

	cipher, err := security.NewCredentialCipher(base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))
	if err != nil {
		t.Fatal(err)
	}
	service := &svc.ServiceContext{
		Config:                 config.Config{AdminApiToken: "test-token"},
		HasDatabse:             true,
		Studio:                 model.NewStudioModel(model.NewSupabaseREST(server.URL, "key")),
		StudioCredentialCipher: cipher,
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/studio/credentials", strings.NewReader(`{"name":"Production token","type":"bearer","data":{"token":"top-secret"}}`))
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()

	AdminCreateStudioCredentialHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "top-secret") || strings.Contains(recorder.Body.String(), "encryptedData") {
		t.Fatalf("secret leaked in response: %s", recorder.Body.String())
	}
	ciphertext, _ := persisted["encryptedData"].(string)
	if ciphertext == "" || strings.Contains(ciphertext, "top-secret") {
		t.Fatalf("credential was not encrypted: %#v", persisted)
	}
	credentialID, _ := persisted["id"].(string)
	if decrypted, err := cipher.DecryptFor(studioCredentialCipherScope(credentialID, "bearer"), ciphertext); err != nil || decrypted["token"] != "top-secret" {
		t.Fatalf("credential scope/decryption failed: data=%#v err=%v", decrypted, err)
	}
}

func TestValidateStudioCredentialData(t *testing.T) {
	t.Parallel()

	valid := []studioCredentialPayload{
		{Name: "Bearer", Type: "bearer", Data: map[string]string{"token": "token"}},
		{Name: "Basic", Type: "basic", Data: map[string]string{"username": "user", "password": "pass"}},
		{Name: "Header", Type: "header", Data: map[string]string{"name": "X-API-Key", "value": "key"}},
		{Name: "Query", Type: "query", Data: map[string]string{"name": "api_key", "value": "key"}},
	}
	for _, payload := range valid {
		if err := validateStudioCredentialPayload(payload); err != nil {
			t.Fatalf("valid payload rejected: %#v err=%v", payload, err)
		}
	}
	invalid := []studioCredentialPayload{
		{Name: "x", Type: "bearer", Data: map[string]string{"token": ""}},
		{Name: "Unknown", Type: "oauth", Data: map[string]string{"token": "x"}},
		{Name: "Header", Type: "header", Data: map[string]string{"name": "Host", "value": "bad"}},
		{Name: "Extra", Type: "bearer", Data: map[string]string{"token": "x", "extra": "bad"}},
	}
	for _, payload := range invalid {
		if err := validateStudioCredentialPayload(payload); err == nil {
			t.Fatalf("invalid payload accepted: %#v", payload)
		}
	}
}
