package model

import (
	"context"
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
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
	LastSeenAt    string  `json:"lastSeenAt"`
	ExpiresAt     string  `json:"expiresAt"`
}

type portfolioChatMessageRow struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	CreatedAt string         `json:"createdAt"`
	Metadata  map[string]any `json:"metadata"`
}

func rowToPortfolioChatSession(row portfolioChatSessionRow) PortfolioChatSession {
	return PortfolioChatSession{
		ID:            row.ID,
		VisitorIDHash: row.VisitorIDHash,
		ThreadID:      row.ThreadID,
		Locale:        row.Locale,
		Title:         row.Title,
		CreatedAt:     timeFromString(row.CreatedAt),
		UpdatedAt:     timeFromString(row.UpdatedAt),
		LastSeenAt:    timeFromString(row.LastSeenAt),
		ExpiresAt:     timeFromString(row.ExpiresAt),
	}
}

func rowToPortfolioChatMessage(row portfolioChatMessageRow) PortfolioChatMessage {
	return PortfolioChatMessage{
		ID:        row.ID,
		SessionID: row.SessionID,
		Role:      row.Role,
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
	values.Set("select", "id,visitorIdHash,threadId,locale,title,createdAt,updatedAt,lastSeenAt,expiresAt")
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
	values.Set("select", "id,visitorIdHash,threadId,locale,title,createdAt,updatedAt,lastSeenAt,expiresAt")
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
	values.Set("select", "id,visitorIdHash,threadId,locale,title,createdAt,updatedAt,lastSeenAt,expiresAt")
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
	values.Set("select", "id,sessionId,role,content,createdAt,metadata")
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

func (m *PortfolioChatMessageModel) Append(ctx context.Context, sessionID, role, content string, metadata map[string]any) (*PortfolioChatMessage, error) {
	body := map[string]any{
		"id":        newID(),
		"sessionId": sessionID,
		"role":      role,
		"content":   content,
		"metadata":  metadata,
	}
	values := url.Values{}
	values.Set("select", "id,sessionId,role,content,createdAt,metadata")
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
