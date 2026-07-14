package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStudioCredentialModelCRUDKeepsCiphertextPrivate(t *testing.T) {
	t.Parallel()

	var created map[string]any
	var revoked map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if r.URL.Query().Get("select") == "id,name,type,status,createdAt,updatedAt" {
				_, _ = w.Write([]byte(`[{"id":"cred-1","name":"API token","type":"bearer","status":"active","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
				return
			}
			_, _ = w.Write([]byte(`[{"id":"cred-1","name":"API token","type":"bearer","status":"active","encryptedData":"v1:ciphertext","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		case http.MethodPost:
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_, _ = w.Write([]byte(`[{"id":"cred-1","name":"API token","type":"bearer","status":"active","encryptedData":"v1:ciphertext","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
		case http.MethodPatch:
			if err := json.NewDecoder(r.Body).Decode(&revoked); err != nil {
				t.Fatal(err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()

	model := NewStudioModel(NewSupabaseREST(server.URL, "key"))
	items, err := model.ListCredentials(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "cred-1" {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	record, err := model.FindCredential(context.Background(), "cred-1")
	if err != nil || record == nil || record.EncryptedData != "v1:ciphertext" {
		t.Fatalf("record=%#v err=%v", record, err)
	}
	createdRecord, err := model.CreateCredential(context.Background(), StudioCredentialInput{
		Name: "API token", Type: "bearer", EncryptedData: "v1:ciphertext",
	})
	if err != nil || createdRecord.ID != "cred-1" || created["encryptedData"] != "v1:ciphertext" {
		t.Fatalf("created=%#v body=%#v err=%v", createdRecord, created, err)
	}
	encoded, err := json.Marshal(createdRecord)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || containsJSONField(encoded, "encryptedData") {
		t.Fatalf("credential ciphertext leaked through JSON: %s", encoded)
	}
	if err := model.DeleteCredential(context.Background(), "cred-1"); err != nil {
		t.Fatal(err)
	}
	if revoked["status"] != "revoked" {
		t.Fatalf("credential delete must revoke instead of physically deleting: %#v", revoked)
	}
}

func containsJSONField(encoded []byte, field string) bool {
	var value map[string]any
	if json.Unmarshal(encoded, &value) != nil {
		return false
	}
	_, exists := value[field]
	return exists
}
