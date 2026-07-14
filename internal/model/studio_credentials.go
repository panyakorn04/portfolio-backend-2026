package model

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type StudioCredential struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type StudioCredentialRecord struct {
	StudioCredential
	EncryptedData string `json:"-"`
}

type StudioCredentialInput struct {
	ID            string
	Name          string
	Type          string
	EncryptedData string
}

type studioCredentialRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Status        string `json:"status"`
	EncryptedData string `json:"encryptedData"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

func credentialFromRow(row studioCredentialRow) StudioCredentialRecord {
	return StudioCredentialRecord{
		StudioCredential: StudioCredential{
			ID: row.ID, Name: row.Name, Type: row.Type, Status: row.Status,
			CreatedAt: timeFromString(row.CreatedAt), UpdatedAt: timeFromString(row.UpdatedAt),
		},
		EncryptedData: row.EncryptedData,
	}
}

func (m *StudioModel) ListCredentials(ctx context.Context) ([]StudioCredential, error) {
	values := url.Values{
		"select": {"id,name,type,status,createdAt,updatedAt"},
		"status": {"eq.active"},
		"order":  {"updatedAt.desc"},
		"limit":  {"100"},
	}
	var rows []studioCredentialRow
	if _, err := m.api.request(ctx, http.MethodGet, "StudioCredential", values, nil, "", &rows); err != nil {
		return nil, err
	}
	items := make([]StudioCredential, 0, len(rows))
	for _, row := range rows {
		items = append(items, credentialFromRow(row).StudioCredential)
	}
	return items, nil
}

func (m *StudioModel) FindCredential(ctx context.Context, id string) (*StudioCredentialRecord, error) {
	values := url.Values{"id": {"eq." + id}, "select": {"*"}, "limit": {"1"}}
	var rows []studioCredentialRow
	if _, err := m.api.request(ctx, http.MethodGet, "StudioCredential", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	item := credentialFromRow(rows[0])
	return &item, nil
}

func NewStudioCredentialID() string { return newID() }

func (m *StudioModel) CreateCredential(ctx context.Context, input StudioCredentialInput) (*StudioCredentialRecord, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := input.ID
	if id == "" {
		id = NewStudioCredentialID()
	}
	body := map[string]any{
		"id": id, "name": input.Name, "type": input.Type, "status": "active",
		"encryptedData": input.EncryptedData, "createdAt": now, "updatedAt": now,
	}
	var rows []studioCredentialRow
	if _, err := m.api.request(ctx, http.MethodPost, "StudioCredential", url.Values{"select": {"*"}}, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	item := credentialFromRow(rows[0])
	return &item, nil
}

func (m *StudioModel) UpdateCredential(ctx context.Context, id string, input StudioCredentialInput) (*StudioCredentialRecord, error) {
	body := map[string]any{
		"name": input.Name, "type": input.Type, "encryptedData": input.EncryptedData,
		"updatedAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	var rows []studioCredentialRow
	values := url.Values{"id": {"eq." + id}, "select": {"*"}}
	if _, err := m.api.request(ctx, http.MethodPatch, "StudioCredential", values, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	item := credentialFromRow(rows[0])
	return &item, nil
}

func (m *StudioModel) DeleteCredential(ctx context.Context, id string) error {
	values := url.Values{"id": {"eq." + id}}
	body := map[string]any{"status": "revoked", "updatedAt": time.Now().UTC().Format(time.RFC3339Nano)}
	_, err := m.api.request(ctx, http.MethodPatch, "StudioCredential", values, body, "return=minimal", nil)
	return err
}
