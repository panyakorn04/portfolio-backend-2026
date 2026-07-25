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
	RunID     *string        `json:"runId"`
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
		RunID:     row.RunID,
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
	values.Set("select", "id,sessionId,role,type,content,createdAt,metadata,runId")
	values.Set("sessionId", "eq."+sessionID)
	if limit > 0 {
		values.Set("order", "createdAt.desc,id.desc")
		values.Set("limit", strconv.Itoa(limit))
	} else {
		values.Set("order", "createdAt.asc,id.asc")
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
	values.Set("select", "id,sessionId,role,type,content,createdAt,metadata,runId")
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

const (
	PortfolioChatOutcomeInserted            = "inserted"
	PortfolioChatOutcomeReplayed            = "replayed"
	PortfolioChatOutcomeIdempotencyConflict = "idempotency_conflict"
	PortfolioChatOutcomeStateConflict       = "state_conflict"
	PortfolioChatOutcomeNotFound            = "not_found"
	PortfolioChatOutcomeUpdated             = "updated"
	PortfolioChatOutcomeHealed              = "healed"
	PortfolioChatOutcomeReplied             = "replied"
	PortfolioChatOutcomeClaimed             = "claimed"
	PortfolioChatOutcomeInProgress          = "in_progress"
)

type PortfolioChatRunClaimInput struct {
	SessionID     string
	VisitorIDHash string
	RunID         string
	UserContent   string
	LeaseOwner    string
	LeaseUntil    time.Time
	OccurredAt    time.Time
}

type PortfolioChatRunClaimResult struct {
	Outcome          string `json:"outcome"`
	AssistantContent string `json:"assistantContent"`
	ModelName        string `json:"modelName"`
}

type PortfolioChatExchangeInput struct {
	SessionID          string
	VisitorIDHash      string
	RunID              string
	LeaseOwner         string
	UserMessageID      string
	AssistantMessageID string
	UserContent        string
	AssistantContent   string
	ModelName          string
	ExpiresAt          time.Time
	MaxMessages        int
	OccurredAt         time.Time
}

type PortfolioChatExchangeResult struct {
	Outcome          string                `json:"outcome"`
	UserMessage      *PortfolioChatMessage `json:"userMessage"`
	AssistantMessage *PortfolioChatMessage `json:"assistantMessage"`
}

type PortfolioChatRequestHumanInput struct {
	SessionID      string
	VisitorIDHash  string
	EventMessageID string
	MaxMessages    int
	OccurredAt     time.Time
}

type PortfolioChatTakeoverReplyInput struct {
	SessionID         string
	TakeoverMessageID string
	ReplyMessageID    string
	ReplyContent      string
	AdminName         string
	MaxMessages       int
	OccurredAt        time.Time
}

type PortfolioChatMutationResult struct {
	Outcome      string                `json:"outcome"`
	Status       string                `json:"status"`
	EventMessage *PortfolioChatMessage `json:"eventMessage,omitempty"`
	ReplyMessage *PortfolioChatMessage `json:"replyMessage,omitempty"`
}

type PortfolioChatRetentionResult struct {
	MessagesDeleted int64 `json:"messagesDeleted"`
	SessionsDeleted int64 `json:"sessionsDeleted"`
}

func (m *PortfolioChatMessageModel) ClaimRun(ctx context.Context, input PortfolioChatRunClaimInput) (*PortfolioChatRunClaimResult, error) {
	body := map[string]any{
		"sessionId": input.SessionID, "visitorIdHash": input.VisitorIDHash, "runId": input.RunID,
		"userContent": input.UserContent, "leaseOwner": input.LeaseOwner,
		"leaseUntil": input.LeaseUntil.UTC(), "occurredAt": input.OccurredAt.UTC(),
	}
	var rows []PortfolioChatRunClaimResult
	if _, err := m.api.request(ctx, http.MethodPost, "rpc/claimPortfolioChatRun", nil, body, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

func (m *PortfolioChatMessageModel) CompleteRun(ctx context.Context, input PortfolioChatExchangeInput) (*PortfolioChatExchangeResult, error) {
	if input.UserMessageID == "" {
		input.UserMessageID = newID()
	}
	if input.AssistantMessageID == "" {
		input.AssistantMessageID = newID()
	}
	body := map[string]any{
		"sessionId": input.SessionID, "visitorIdHash": input.VisitorIDHash, "runId": input.RunID,
		"leaseOwner":    input.LeaseOwner,
		"userMessageId": input.UserMessageID, "assistantMessageId": input.AssistantMessageID,
		"userContent": input.UserContent, "assistantContent": input.AssistantContent, "modelName": input.ModelName,
		"expiresAt": input.ExpiresAt.UTC(), "maxMessages": input.MaxMessages, "occurredAt": input.OccurredAt.UTC(),
	}
	var rows []PortfolioChatExchangeResult
	if _, err := m.api.request(ctx, http.MethodPost, "rpc/completePortfolioChatRun", nil, body, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

func (m *PortfolioChatMessageModel) ReleaseRun(ctx context.Context, sessionID, visitorIDHash, runID, leaseOwner string) error {
	body := map[string]any{
		"sessionId": sessionID, "visitorIdHash": visitorIDHash, "runId": runID, "leaseOwner": leaseOwner,
	}
	var released bool
	_, err := m.api.request(ctx, http.MethodPost, "rpc/releasePortfolioChatRun", nil, body, "", &released)
	return err
}

func (m *PortfolioChatSessionModel) RequestHuman(ctx context.Context, input PortfolioChatRequestHumanInput) (*PortfolioChatMutationResult, error) {
	if input.EventMessageID == "" {
		input.EventMessageID = newID()
	}
	body := map[string]any{
		"sessionId": input.SessionID, "visitorIdHash": input.VisitorIDHash, "eventMessageId": input.EventMessageID,
		"maxMessages": input.MaxMessages, "occurredAt": input.OccurredAt.UTC(),
	}
	return m.callChatMutation(ctx, "rpc/requestPortfolioChatHuman", body)
}

func (m *PortfolioChatSessionModel) TakeoverAndReply(ctx context.Context, input PortfolioChatTakeoverReplyInput) (*PortfolioChatMutationResult, error) {
	if input.TakeoverMessageID == "" {
		input.TakeoverMessageID = newID()
	}
	if input.ReplyMessageID == "" {
		input.ReplyMessageID = newID()
	}
	body := map[string]any{
		"sessionId": input.SessionID, "takeoverMessageId": input.TakeoverMessageID, "replyMessageId": input.ReplyMessageID,
		"replyContent": input.ReplyContent, "adminName": input.AdminName, "maxMessages": input.MaxMessages,
		"occurredAt": input.OccurredAt.UTC(),
	}
	return m.callChatMutation(ctx, "rpc/takeoverAndReplyPortfolioChat", body)
}

func (m *PortfolioChatSessionModel) callChatMutation(ctx context.Context, path string, body map[string]any) (*PortfolioChatMutationResult, error) {
	var rows []PortfolioChatMutationResult
	if _, err := m.api.request(ctx, http.MethodPost, path, nil, body, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	return &rows[0], nil
}

func (m *PortfolioChatSessionModel) PruneRetention(ctx context.Context, maxMessages int, expiredBefore time.Time, batchSize int) (PortfolioChatRetentionResult, error) {
	body := map[string]any{"maxMessages": maxMessages, "expiredBefore": expiredBefore.UTC(), "batchSize": batchSize}
	var rows []PortfolioChatRetentionResult
	if _, err := m.api.request(ctx, http.MethodPost, "rpc/prunePortfolioChatRetention", nil, body, "", &rows); err != nil {
		return PortfolioChatRetentionResult{}, err
	}
	if len(rows) == 0 {
		return PortfolioChatRetentionResult{}, ErrNotFound
	}
	return rows[0], nil
}
