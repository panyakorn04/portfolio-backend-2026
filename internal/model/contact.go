package model

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ContactInquiryModel struct{ api *SupabaseREST }

func NewContactInquiryModel(api *SupabaseREST) *ContactInquiryModel {
	return &ContactInquiryModel{api: api}
}

type contactInquiryRow struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Email        string  `json:"email"`
	Company      *string `json:"company"`
	Subject      string  `json:"subject"`
	Message      string  `json:"message"`
	Locale       string  `json:"locale"`
	DeliveryMode string  `json:"deliveryMode"`
	Status       string  `json:"status"`
	InternalNote *string `json:"internalNote"`
	HandledAt    *string `json:"handledAt"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

type contactActivityRow struct {
	ID               string  `json:"id"`
	InquiryID        string  `json:"inquiryId"`
	ActorType        string  `json:"actorType"`
	ActorLabel       string  `json:"actorLabel"`
	EventType        string  `json:"eventType"`
	StatusFrom       *string `json:"statusFrom"`
	StatusTo         *string `json:"statusTo"`
	InternalNoteFrom *string `json:"internalNoteFrom"`
	InternalNoteTo   *string `json:"internalNoteTo"`
	CreatedAt        string  `json:"createdAt"`
}

func convertContactRow(row contactInquiryRow) ContactInquiry {
	return ContactInquiry{ID: row.ID, Name: row.Name, Email: row.Email, Company: row.Company, Subject: row.Subject, Message: row.Message,
		Locale: row.Locale, DeliveryMode: row.DeliveryMode, Status: row.Status, InternalNote: row.InternalNote, HandledAt: timePtrFromString(row.HandledAt), CreatedAt: timeFromString(row.CreatedAt), UpdatedAt: timeFromString(row.UpdatedAt)}
}

func convertActivityRow(row contactActivityRow) ContactInquiryActivity {
	return ContactInquiryActivity{ID: row.ID, ActorType: row.ActorType, ActorLabel: row.ActorLabel, EventType: row.EventType,
		StatusFrom: row.StatusFrom, StatusTo: row.StatusTo, InternalNoteFrom: row.InternalNoteFrom, InternalNoteTo: row.InternalNoteTo, CreatedAt: timeFromString(row.CreatedAt)}
}

// Create inserts a contact inquiry plus the initial "created" activity.
func (m *ContactInquiryModel) Create(ctx context.Context, in ContactInquiry) (*ContactInquiry, error) {
	id := newID()
	body := map[string]any{"id": id, "name": in.Name, "email": in.Email, "company": in.Company, "subject": in.Subject, "message": in.Message, "locale": in.Locale, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	var rows []contactInquiryRow
	if _, err := m.api.request(ctx, http.MethodPost, "ContactInquiry", url.Values{}, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	activity := map[string]any{"id": newID(), "inquiryId": id, "actorType": "system", "actorLabel": "contact_form", "eventType": "created", "statusTo": "new"}
	if _, err := m.api.request(ctx, http.MethodPost, "ContactInquiryActivity", url.Values{}, activity, "", nil); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	created := convertContactRow(rows[0])
	return &created, nil
}

func (m *ContactInquiryModel) UpdateDeliveryMode(ctx context.Context, id, deliveryMode string) error {
	values := url.Values{}
	values.Set("id", "eq."+id)
	_, err := m.api.request(ctx, http.MethodPatch, "ContactInquiry", values, map[string]any{"deliveryMode": deliveryMode, "updatedAt": time.Now().UTC().Format(time.RFC3339)}, "", nil)
	return err
}

func (m *ContactInquiryModel) FindByID(ctx context.Context, id string) (*ContactInquiry, error) {
	values := url.Values{}
	values.Set("id", "eq."+id)
	values.Set("limit", "1")
	var rows []contactInquiryRow
	if _, err := m.api.request(ctx, http.MethodGet, "ContactInquiry", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	c := convertContactRow(rows[0])
	return &c, nil
}

func (m *ContactInquiryModel) FindDetailByID(ctx context.Context, id string) (*ContactInquiryDetail, error) {
	inquiry, err := m.FindByID(ctx, id)
	if err != nil || inquiry == nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("inquiryId", "eq."+id)
	values.Set("order", "createdAt.desc")
	var rows []contactActivityRow
	if _, err := m.api.request(ctx, http.MethodGet, "ContactInquiryActivity", values, nil, "", &rows); err != nil {
		return nil, err
	}
	activities := make([]ContactInquiryActivity, 0, len(rows))
	for _, row := range rows {
		activities = append(activities, convertActivityRow(row))
	}
	return &ContactInquiryDetail{ContactInquiry: *inquiry, Activities: activities}, nil
}

type ListInquiriesInput struct {
	Limit  int
	Page   int
	Status string
	Query  string
}
type ListInquiriesResult struct {
	Items      []ContactInquiry `json:"items"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	TotalPages int              `json:"totalPages"`
}

func (m *ContactInquiryModel) List(ctx context.Context, in ListInquiriesInput) (*ListInquiriesResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	page := in.Page
	if page <= 0 {
		page = 1
	}
	values := url.Values{}
	values.Set("order", "createdAt.desc")
	values.Set("limit", fmt.Sprintf("%d", limit))
	values.Set("offset", fmt.Sprintf("%d", (page-1)*limit))
	if in.Status != "" && in.Status != "all" {
		values.Set("status", "eq."+in.Status)
	}
	if q := strings.TrimSpace(in.Query); q != "" {
		like := "*" + strings.ReplaceAll(q, ",", "") + "*"
		values.Set("or", fmt.Sprintf("(name.ilike.%s,email.ilike.%s,company.ilike.%s,subject.ilike.%s,message.ilike.%s)", like, like, like, like, like))
	}
	var rows []contactInquiryRow
	resp, err := m.api.request(ctx, http.MethodGet, "ContactInquiry", values, nil, "count=exact", &rows)
	if err != nil {
		return nil, err
	}
	items := make([]ContactInquiry, 0, len(rows))
	for _, row := range rows {
		items = append(items, convertContactRow(row))
	}
	total := exactCount(resp, len(items))
	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}
	return &ListInquiriesResult{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages}, nil
}

type UpdateInquiryInput struct {
	Status       string
	InternalNote *string
	ActorType    string
	ActorLabel   string
}

func (m *ContactInquiryModel) Update(ctx context.Context, id string, in UpdateInquiryInput) (*ContactInquiry, error) {
	current, err := m.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrNotFound
	}
	statusChanged := current.Status != in.Status
	noteChanged := !equalStringPtr(current.InternalNote, in.InternalNote)
	var handledAt any = nil
	if in.Status == "handled" {
		handledAt = time.Now().UTC().Format(time.RFC3339)
	}
	values := url.Values{}
	values.Set("id", "eq."+id)
	body := map[string]any{"status": in.Status, "internalNote": in.InternalNote, "handledAt": handledAt, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	var rows []contactInquiryRow
	if _, err := m.api.request(ctx, http.MethodPatch, "ContactInquiry", values, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	if statusChanged || noteChanged {
		eventType := "note_updated"
		switch {
		case statusChanged && noteChanged:
			eventType = "status_and_note_updated"
		case statusChanged:
			eventType = "status_updated"
		}
		activity := map[string]any{"id": newID(), "inquiryId": id, "actorType": in.ActorType, "actorLabel": in.ActorLabel, "eventType": eventType, "statusFrom": current.Status, "statusTo": in.Status, "internalNoteFrom": current.InternalNote, "internalNoteTo": in.InternalNote}
		if _, err := m.api.request(ctx, http.MethodPost, "ContactInquiryActivity", url.Values{}, activity, "", nil); err != nil {
			return nil, err
		}
	}
	updated := convertContactRow(rows[0])
	return &updated, nil
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
