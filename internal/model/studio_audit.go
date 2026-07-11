package model

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type StudioAuditLog struct {
	ID           string         `json:"id"`
	ActorType    string         `json:"actorType"`
	ActorID      string         `json:"actorId,omitempty"`
	ActorLabel   string         `json:"actorLabel"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	FromStatus   string         `json:"fromStatus,omitempty"`
	ToStatus     string         `json:"toStatus,omitempty"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
}

type StudioAuditInput struct {
	ActorType, ActorID, ActorLabel, Action, ResourceType, ResourceID, FromStatus, ToStatus string
	Metadata                                                                               map[string]any
}

type studioAuditRow struct {
	ID           string         `json:"id"`
	ActorType    string         `json:"actorType"`
	ActorID      *string        `json:"actorId"`
	ActorLabel   string         `json:"actorLabel"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   string         `json:"resourceId"`
	FromStatus   *string        `json:"fromStatus"`
	ToStatus     *string        `json:"toStatus"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    string         `json:"createdAt"`
}

func auditFromRow(r studioAuditRow) StudioAuditLog {
	v := StudioAuditLog{ID: r.ID, ActorType: r.ActorType, ActorLabel: r.ActorLabel, Action: r.Action, ResourceType: r.ResourceType, ResourceID: r.ResourceID, Metadata: r.Metadata, CreatedAt: timeFromString(r.CreatedAt)}
	if r.ActorID != nil {
		v.ActorID = *r.ActorID
	}
	if r.FromStatus != nil {
		v.FromStatus = *r.FromStatus
	}
	if r.ToStatus != nil {
		v.ToStatus = *r.ToStatus
	}
	return v
}

func (m *StudioModel) CreateAudit(ctx context.Context, in StudioAuditInput) (*StudioAuditLog, error) {
	body := map[string]any{"id": newID(), "actorType": in.ActorType, "actorLabel": in.ActorLabel, "action": in.Action, "resourceType": in.ResourceType, "resourceId": in.ResourceID, "metadata": in.Metadata}
	if in.ActorID != "" {
		body["actorId"] = in.ActorID
	}
	if in.FromStatus != "" {
		body["fromStatus"] = in.FromStatus
	}
	if in.ToStatus != "" {
		body["toStatus"] = in.ToStatus
	}
	var rows []studioAuditRow
	if _, err := m.api.request(ctx, http.MethodPost, "StudioAuditLog", url.Values{"select": {"*"}}, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	out := auditFromRow(rows[0])
	return &out, nil
}

func (m *StudioModel) ListAudits(ctx context.Context, limit int) ([]StudioAuditLog, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var rows []studioAuditRow
	v := url.Values{"select": {"*"}, "order": {"createdAt.desc"}, "limit": {strconv.Itoa(limit)}}
	if _, err := m.api.request(ctx, http.MethodGet, "StudioAuditLog", v, nil, "", &rows); err != nil {
		return nil, err
	}
	out := make([]StudioAuditLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, auditFromRow(row))
	}
	return out, nil
}
