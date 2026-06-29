package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ContactInquiryModel struct {
	db *sql.DB
}

func NewContactInquiryModel(db *sql.DB) *ContactInquiryModel {
	return &ContactInquiryModel{db: db}
}

const contactInquiryColumns = `"id", "name", "email", "company", "subject", "message", "locale",
	"deliveryMode", "status", "internalNote", "handledAt", "createdAt", "updatedAt"`

func scanContactInquiry(row interface {
	Scan(dest ...any) error
}) (*ContactInquiry, error) {
	var c ContactInquiry
	err := row.Scan(
		&c.ID, &c.Name, &c.Email, &c.Company, &c.Subject, &c.Message, &c.Locale,
		&c.DeliveryMode, &c.Status, &c.InternalNote, &c.HandledAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// Create inserts a contact inquiry plus the initial "created" activity in one transaction.
func (m *ContactInquiryModel) Create(ctx context.Context, in ContactInquiry) (*ContactInquiry, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	const insertInquiry = `INSERT INTO "ContactInquiry"
		("id", "name", "email", "company", "subject", "message", "locale", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, $6, $7, now()) RETURNING ` + contactInquiryColumns

	row := tx.QueryRowContext(ctx, insertInquiry,
		newID(), in.Name, in.Email, in.Company, in.Subject, in.Message, in.Locale)

	created, err := scanContactInquiry(row)
	if err != nil {
		return nil, err
	}

	const insertActivity = `INSERT INTO "ContactInquiryActivity"
		("id", "inquiryId", "actorType", "actorLabel", "eventType", "statusTo")
		VALUES ($1, $2, 'system', 'contact_form', 'created', 'new')`

	if _, err := tx.ExecContext(ctx, insertActivity, newID(), created.ID); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (m *ContactInquiryModel) UpdateDeliveryMode(ctx context.Context, id, deliveryMode string) error {
	const query = `UPDATE "ContactInquiry" SET "deliveryMode" = $1, "updatedAt" = now() WHERE "id" = $2`
	_, err := m.db.ExecContext(ctx, query, deliveryMode, id)
	return err
}

func (m *ContactInquiryModel) FindByID(ctx context.Context, id string) (*ContactInquiry, error) {
	const query = `SELECT ` + contactInquiryColumns + ` FROM "ContactInquiry" WHERE "id" = $1`
	c, err := scanContactInquiry(m.db.QueryRowContext(ctx, query, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (m *ContactInquiryModel) FindDetailByID(ctx context.Context, id string) (*ContactInquiryDetail, error) {
	inquiry, err := m.FindByID(ctx, id)
	if err != nil || inquiry == nil {
		return nil, err
	}

	const activityQuery = `SELECT "id", "actorType", "actorLabel", "eventType",
		"statusFrom", "statusTo", "internalNoteFrom", "internalNoteTo", "createdAt"
		FROM "ContactInquiryActivity" WHERE "inquiryId" = $1 ORDER BY "createdAt" DESC`

	rows, err := m.db.QueryContext(ctx, activityQuery, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	activities := make([]ContactInquiryActivity, 0)
	for rows.Next() {
		var a ContactInquiryActivity
		if err := rows.Scan(&a.ID, &a.ActorType, &a.ActorLabel, &a.EventType,
			&a.StatusFrom, &a.StatusTo, &a.InternalNoteFrom, &a.InternalNoteTo, &a.CreatedAt); err != nil {
			return nil, err
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &ContactInquiryDetail{ContactInquiry: *inquiry, Activities: activities}, nil
}

type ListInquiriesInput struct {
	Limit  int
	Page   int
	Status string // "all" or specific status
	Query  string
}

type ListInquiriesResult struct {
	Items      []ContactInquiry `json:"items"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	Limit      int              `json:"limit"`
	TotalPages int              `json:"totalPages"`
}

func buildInquiryWhere(in ListInquiriesInput) (string, []any) {
	var clauses []string
	var args []any
	idx := 1

	if in.Status != "" && in.Status != "all" {
		clauses = append(clauses, fmt.Sprintf(`"status" = $%d`, idx))
		args = append(args, in.Status)
		idx++
	}

	if q := strings.TrimSpace(in.Query); q != "" {
		like := "%" + q + "%"
		clauses = append(clauses, fmt.Sprintf(
			`("name" ILIKE $%d OR "email" ILIKE $%d OR "company" ILIKE $%d OR "subject" ILIKE $%d OR "message" ILIKE $%d)`,
			idx, idx, idx, idx, idx))
		args = append(args, like)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
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

	where, args := buildInquiryWhere(in)

	countQuery := `SELECT count(*) FROM "ContactInquiry" ` + where
	var total int
	if err := m.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	listQuery := fmt.Sprintf(
		`SELECT %s FROM "ContactInquiry" %s ORDER BY "createdAt" DESC LIMIT $%d OFFSET $%d`,
		contactInquiryColumns, where, len(args)+1, len(args)+2)
	listArgs := append(append([]any{}, args...), limit, (page-1)*limit)

	rows, err := m.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ContactInquiry, 0)
	for rows.Next() {
		c, err := scanContactInquiry(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	return &ListInquiriesResult{
		Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages,
	}, nil
}

type UpdateInquiryInput struct {
	Status       string
	InternalNote *string
	ActorType    string
	ActorLabel   string
}

// Update changes status/note and logs an activity only when something changed.
// Returns ErrNotFound when the inquiry does not exist.
func (m *ContactInquiryModel) Update(ctx context.Context, id string, in UpdateInquiryInput) (*ContactInquiry, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus string
	var currentNote *string
	err = tx.QueryRowContext(ctx,
		`SELECT "status", "internalNote" FROM "ContactInquiry" WHERE "id" = $1`, id).
		Scan(&currentStatus, &currentNote)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	statusChanged := currentStatus != in.Status
	noteChanged := !equalStringPtr(currentNote, in.InternalNote)

	var handledAt *time.Time
	if in.Status == "handled" {
		now := time.Now()
		handledAt = &now
	}

	const updateQuery = `UPDATE "ContactInquiry"
		SET "status" = $1, "internalNote" = $2, "handledAt" = $3, "updatedAt" = now()
		WHERE "id" = $4 RETURNING ` + contactInquiryColumns

	updated, err := scanContactInquiry(tx.QueryRowContext(ctx, updateQuery,
		in.Status, in.InternalNote, handledAt, id))
	if err != nil {
		return nil, err
	}

	if statusChanged || noteChanged {
		eventType := "note_updated"
		switch {
		case statusChanged && noteChanged:
			eventType = "status_and_note_updated"
		case statusChanged:
			eventType = "status_updated"
		}

		const insertActivity = `INSERT INTO "ContactInquiryActivity"
			("id", "inquiryId", "actorType", "actorLabel", "eventType",
			 "statusFrom", "statusTo", "internalNoteFrom", "internalNoteTo")
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

		if _, err := tx.ExecContext(ctx, insertActivity,
			newID(), id, in.ActorType, in.ActorLabel, eventType,
			currentStatus, in.Status, currentNote, in.InternalNote); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return updated, nil
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
