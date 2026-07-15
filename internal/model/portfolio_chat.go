package model

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

type PortfolioChatSessionModel struct{ api *SupabaseREST }

func NewPortfolioChatSessionModel(api *SupabaseREST) *PortfolioChatSessionModel {
	return &PortfolioChatSessionModel{api: api}
}

type PortfolioChatMessageModel struct{ api *SupabaseREST }

func NewPortfolioChatMessageModel(api *SupabaseREST) *PortfolioChatMessageModel {
	return &PortfolioChatMessageModel{api: api}
}

type portfolioChatSessionRow struct {
	ID            string  `json:"id"`
	VisitorIDHash string  `json:"visitorIdHash"`
	ThreadID      string  `json:"threadId"`
	Locale        string  `json:"locale"`
	Title         *string `json:"title"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	LastSeenAt    string  `json:"lastSeenAt"`
	ExpiresAt     string  `json:"expiresAt"`
}

type portfolioChatMessageRow struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	Role      string         `json:"role"`
	Type      string         `json:"type"`
	Content   string         `json:"content"`
	CreatedAt string         `json:"createdAt"`
	Metadata  map[string]any `json:"metadata"`
}

func rowToPortfolioChatSession(row portfolioChatSessionRow) PortfolioChatSession {
	status := row.Status
	if status == "" {
		status = "active"
	}
	return PortfolioChatSession{
		ID:            row.ID,
		VisitorIDHash: row.VisitorIDHash,
		ThreadID:      row.ThreadID,
		Locale:        row.Locale,
		Title:         row.Title,
		Status:        status,
		CreatedAt:     timeFromString(row.CreatedAt),
		UpdatedAt:     timeFromString(row.UpdatedAt),
		LastSeenAt:    timeFromString(row.LastSeenAt),
		ExpiresAt:     timeFromString(row.ExpiresAt),
	}
}

func rowToPortfolioChatMessage(row portfolioChatMessageRow) PortfolioChatMessage {
	msgType := row.Type
	if msgType == "" {
		msgType = "chat"
	}
	return PortfolioChatMessage{
		ID:        row.ID,
		SessionID: row.SessionID,
		Role:      row.Role,
		Type:      msgType,
		Content:   row.Content,
		CreatedAt: timeFromString(row.CreatedAt),
		Metadata:  row.Metadata,
	}
}

func (m *PortfolioChatSessionModel) Create(ctx context.Context, visitorHash, threadID, locale string, title *string, expiresAt time.Time) (*PortfolioChatSession, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	body := map[string]any{
		"id":            newID(),
		"visitorIdHash": visitorHash,
		"threadId":      threadID,
		"locale":        locale,
		"updatedAt":     now,
		"lastSeenAt":    now,
		"expiresAt":     expiresAt.UTC().Format(time.RFC3339),
	}
	if title != nil {
		body["title"] = *title
	}
	values := url.Values{}
	values.Set("select", "id,visitorIdHash,threadId,locale,title,status,createdAt,updatedAt,lastSeenAt,expiresAt")
	var rows []portfolioChatSessionRow
	if _, err := m.api.request(ctx, http.MethodPost, "PortfolioChatSession", values, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	session := rowToPortfolioChatSession(rows[0])
	return &session, nil
}

func (m *PortfolioChatSessionModel) FindLatestActiveByVisitorHash(ctx context.Context, visitorHash string, now time.Time) (*PortfolioChatSession, error) {
	values := url.Values{}
	values.Set("select", "id,visitorIdHash,threadId,locale,title,status,createdAt,updatedAt,lastSeenAt,expiresAt")
	values.Set("visitorIdHash", "eq."+visitorHash)
	values.Set("expiresAt", "gt."+now.UTC().Format(time.RFC3339))
	values.Set("order", "lastSeenAt.desc")
	values.Set("limit", "1")
	var rows []portfolioChatSessionRow
	if _, err := m.api.request(ctx, http.MethodGet, "PortfolioChatSession", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	session := rowToPortfolioChatSession(rows[0])
	return &session, nil
}

func (m *PortfolioChatSessionModel) FindByIDForVisitorHash(ctx context.Context, id, visitorHash string, now time.Time) (*PortfolioChatSession, error) {
	values := url.Values{}
	values.Set("select", "id,visitorIdHash,threadId,locale,title,status,createdAt,updatedAt,lastSeenAt,expiresAt")
	values.Set("id", "eq."+id)
	values.Set("visitorIdHash", "eq."+visitorHash)
	values.Set("expiresAt", "gt."+now.UTC().Format(time.RFC3339))
	values.Set("limit", "1")
	var rows []portfolioChatSessionRow
	if _, err := m.api.request(ctx, http.MethodGet, "PortfolioChatSession", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	session := rowToPortfolioChatSession(rows[0])
	return &session, nil
}

func (m *PortfolioChatSessionModel) Touch(ctx context.Context, id string, expiresAt time.Time) error {
	now := time.Now().UTC().Format(time.RFC3339)
	values := url.Values{}
	values.Set("id", "eq."+id)
	body := map[string]any{"updatedAt": now, "lastSeenAt": now, "expiresAt": expiresAt.UTC().Format(time.RFC3339)}
	_, err := m.api.request(ctx, http.MethodPatch, "PortfolioChatSession", values, body, "", nil)
	return err
}

func (m *PortfolioChatSessionModel) DeleteByIDForVisitorHash(ctx context.Context, id, visitorHash string) error {
	values := url.Values{}
	values.Set("id", "eq."+id)
	values.Set("visitorIdHash", "eq."+visitorHash)
	_, err := m.api.request(ctx, http.MethodDelete, "PortfolioChatSession", values, nil, "", nil)
	return err
}

func (m *PortfolioChatMessageModel) ListForSession(ctx context.Context, sessionID string, limit int) ([]PortfolioChatMessage, error) {
	values := url.Values{}
	values.Set("select", "id,sessionId,role,type,content,createdAt,metadata")
	values.Set("sessionId", "eq."+sessionID)
	if limit > 0 {
		values.Set("order", "createdAt.desc")
		values.Set("limit", strconv.Itoa(limit))
	} else {
		values.Set("order", "createdAt.asc")
	}
	var rows []portfolioChatMessageRow
	if _, err := m.api.request(ctx, http.MethodGet, "PortfolioChatMessage", values, nil, "", &rows); err != nil {
		return nil, err
	}
	items := make([]PortfolioChatMessage, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToPortfolioChatMessage(row))
	}
	if limit > 0 {
		slices.Reverse(items)
	}
	return items, nil
}

func (m *PortfolioChatSessionModel) FindByID(ctx context.Context, id string) (*PortfolioChatSession, error) {
	values := url.Values{}
	values.Set("select", "id,visitorIdHash,threadId,locale,title,status,createdAt,updatedAt,lastSeenAt,expiresAt")
	values.Set("id", "eq."+id)
	values.Set("limit", "1")
	var rows []portfolioChatSessionRow
	if _, err := m.api.request(ctx, http.MethodGet, "PortfolioChatSession", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	session := rowToPortfolioChatSession(rows[0])
	return &session, nil
}

func (m *PortfolioChatSessionModel) ListAll(ctx context.Context, statusFilter string, limit, offset int) ([]PortfolioChatSession, int, error) {
	values := url.Values{}
	values.Set("select", "id,visitorIdHash,threadId,locale,title,status,createdAt,updatedAt,lastSeenAt,expiresAt")
	if statusFilter != "" {
		values.Set("status", "eq."+statusFilter)
	}
	values.Set("order", "updatedAt.desc")
	if limit > 0 {
		values.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		values.Set("offset", strconv.Itoa(offset))
	}
	var rows []portfolioChatSessionRow
	resp, err := m.api.request(ctx, http.MethodGet, "PortfolioChatSession", values, nil, "", &rows)
	if err != nil {
		return nil, 0, err
	}
	items := make([]PortfolioChatSession, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToPortfolioChatSession(row))
	}
	total := len(rows)
	if total > 0 {
		if contentRange := resp.Header.Get("Content-Range"); contentRange != "" {
			if _, err := fmt.Sscanf(contentRange, "*/%d", &total); err != nil {
				total = len(rows)
			}
		}
	}
	return items, total, nil
}

func (m *PortfolioChatSessionModel) UpdateStatus(ctx context.Context, id, status string) error {
	values := url.Values{}
	values.Set("id", "eq."+id)
	body := map[string]any{"status": status}
	_, err := m.api.request(ctx, http.MethodPatch, "PortfolioChatSession", values, body, "", nil)
	return err
}

func (m *PortfolioChatMessageModel) Append(ctx context.Context, sessionID, role, msgType, content string, metadata map[string]any) (*PortfolioChatMessage, error) {
	if msgType == "" {
		msgType = "chat"
	}
	body := map[string]any{
		"id":        newID(),
		"sessionId": sessionID,
		"role":      role,
		"type":      msgType,
		"content":   content,
		"metadata":  metadata,
	}
	values := url.Values{}
	values.Set("select", "id,sessionId,role,type,content,createdAt,metadata")
	var rows []portfolioChatMessageRow
	if _, err := m.api.request(ctx, http.MethodPost, "PortfolioChatMessage", values, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	message := rowToPortfolioChatMessage(rows[0])
	return &message, nil
}
