package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const (
	portfolioVisitorCookieName      = "portfolio_visitor_id"
	portfolioVisitorCookieMaxAge    = 180 * 24 * 60 * 60
	defaultPortfolioChatTTLHours    = 90 * 24
	defaultPortfolioChatMaxMessages = 100
)

type portfolioChatSessionResponse struct {
	Session  *portfolioChatSessionDTO  `json:"session"`
	Messages []portfolioChatMessageDTO `json:"messages"`
}

type portfolioChatSessionDTO struct {
	ID        string    `json:"id"`
	ThreadID  string    `json:"threadId"`
	Locale    string    `json:"locale"`
	Title     *string   `json:"title"`
	UpdatedAt time.Time `json:"updatedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type portfolioChatMessageDTO struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type portfolioChatNewSessionRequest struct {
	Title  string `json:"title"`
	Locale string `json:"locale,omitempty"`
}

func PortfolioAssistantCurrentSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return PortfolioAssistantLatestSessionHandler(svcCtx)
}

func PortfolioAssistantLatestSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requirePortfolioChatDatabase(w, svcCtx) {
			return
		}

		visitorHash, ok := portfolioVisitorHash(w, r, svcCtx)
		if !ok {
			response.Error(w, http.StatusServiceUnavailable, "Unable to initialize chat session.")
			return
		}

		now := time.Now().UTC()
		session, err := svcCtx.PortfolioChatSessions.FindLatestActiveByVisitorHash(r.Context(), visitorHash, now)
		if err != nil {
			observability.Error(r.Context(), "portfolio_chat.session.current_lookup_failed", "Portfolio chat current session lookup failed", err)
			response.Error(w, http.StatusServiceUnavailable, "Unable to load chat session.")
			return
		}
		if session == nil {
			response.Ok(w, http.StatusOK, portfolioEmptySessionResponse())
			return
		}

		messages, err := svcCtx.PortfolioChatMessages.ListForSession(r.Context(), session.ID, portfolioChatMaxStoredMessages(svcCtx))
		if err != nil {
			observability.Error(r.Context(), "portfolio_chat.messages.lookup_failed", "Portfolio chat message lookup failed", err)
			response.Error(w, http.StatusServiceUnavailable, "Unable to load chat messages.")
			return
		}
		response.Ok(w, http.StatusOK, portfolioSessionResponse(*session, messages))
	}
}

func PortfolioAssistantNewSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requirePortfolioChatDatabase(w, svcCtx) {
			return
		}

		visitorHash, ok := portfolioVisitorHash(w, r, svcCtx)
		if !ok {
			response.Error(w, http.StatusServiceUnavailable, "Unable to initialize chat session.")
			return
		}

		var payload portfolioChatNewSessionRequest
		if !decodeJSON(w, r, &payload) {
			return
		}
		title := strings.TrimSpace(payload.Title)
		if title == "" {
			response.Error(w, http.StatusBadRequest, "Title is required.")
			return
		}

		session, err := createPortfolioChatSession(r, svcCtx, visitorHash, &title, payload.Locale)
		if err != nil {
			observability.Error(r.Context(), "portfolio_chat.session.create_failed", "Portfolio chat session creation failed", err)
			response.Error(w, http.StatusServiceUnavailable, "Unable to create chat session.")
			return
		}
		response.Ok(w, http.StatusCreated, portfolioSessionResponse(*session, nil))
	}
}

func PortfolioAssistantRequestHumanHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requirePortfolioChatDatabase(w, svcCtx) {
			return
		}

		visitorHash, ok := portfolioVisitorHash(w, r, svcCtx)
		if !ok {
			response.Error(w, http.StatusServiceUnavailable, "Unable to initialize chat session.")
			return
		}

		sessionID := strings.TrimSpace(pathParam(r, "id"))
		if sessionID == "" {
			response.Error(w, http.StatusBadRequest, "Session id is required.")
			return
		}

		session, err := svcCtx.PortfolioChatSessions.FindByIDForVisitorHash(r.Context(), sessionID, visitorHash, time.Now().UTC())
		if err != nil {
			observability.Error(r.Context(), "portfolio_chat.handoff.session_lookup_failed", "Portfolio chat handoff session lookup failed", err)
			response.Error(w, http.StatusServiceUnavailable, "Unable to load chat session.")
			return
		}
		if session == nil {
			response.Error(w, http.StatusNotFound, "Chat session not found.")
			return
		}

		if err := svcCtx.PortfolioChatSessions.UpdateStatus(r.Context(), sessionID, "pending_human"); err != nil {
			observability.Error(r.Context(), "portfolio_chat.handoff.status_update_failed", "Portfolio chat handoff status update failed", err)
			response.Error(w, http.StatusServiceUnavailable, "Unable to request human contact.")
			return
		}

		if _, err := svcCtx.PortfolioChatMessages.Append(r.Context(), sessionID, "system", "request_human", "Visitor requested human contact", map[string]any{"source": "portfolio-widget"}); err != nil {
			observability.Error(r.Context(), "portfolio_chat.handoff.message_append_failed", "Portfolio chat handoff message persistence failed", err)
		}

		response.Ok(w, http.StatusOK, map[string]any{"status": "pending_human"})
	}
}

func PortfolioAssistantDeleteSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requirePortfolioChatDatabase(w, svcCtx) {
			return
		}

		visitorHash, ok := portfolioVisitorHash(w, r, svcCtx)
		if !ok {
			response.Error(w, http.StatusServiceUnavailable, "Unable to initialize chat session.")
			return
		}
		sessionID := strings.TrimSpace(pathParam(r, "id"))
		if sessionID == "" {
			response.Error(w, http.StatusBadRequest, "Session id is required.")
			return
		}
		if err := svcCtx.PortfolioChatSessions.DeleteByIDForVisitorHash(r.Context(), sessionID, visitorHash); err != nil {
			observability.Error(r.Context(), "portfolio_chat.session.delete_failed", "Portfolio chat session deletion failed", err)
			response.Error(w, http.StatusServiceUnavailable, "Unable to delete chat session.")
			return
		}
		response.Ok(w, http.StatusOK, map[string]any{"deleted": true})
	}
}

func requirePortfolioChatDatabase(w http.ResponseWriter, svcCtx *svc.ServiceContext) bool {
	if !svcCtx.HasDatabse || svcCtx.PortfolioChatSessions == nil || svcCtx.PortfolioChatMessages == nil {
		response.Error(w, http.StatusServiceUnavailable, "Portfolio chat sessions are not configured yet.")
		return false
	}
	return true
}

func createPortfolioChatSession(r *http.Request, svcCtx *svc.ServiceContext, visitorHash string, title *string, localeOverride string) (*model.PortfolioChatSession, error) {
	locale := strings.ToLower(strings.TrimSpace(localeOverride))
	if locale == "" {
		locale = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("locale")))
	}
	if locale != "th" {
		locale = "en"
	}
	threadID := fmt.Sprintf("portfolio-widget-%d", time.Now().UnixNano())
	return svcCtx.PortfolioChatSessions.Create(r.Context(), visitorHash, threadID, locale, title, portfolioChatExpiresAt(svcCtx))
}

func portfolioSessionResponse(session model.PortfolioChatSession, messages []model.PortfolioChatMessage) portfolioChatSessionResponse {
	items := make([]portfolioChatMessageDTO, 0, len(messages))
	for _, message := range messages {
		items = append(items, portfolioChatMessageDTO{
			ID:        message.ID,
			Role:      message.Role,
			Text:      message.Content,
			CreatedAt: message.CreatedAt,
		})
	}
	sessionDTO := portfolioChatSessionDTO{
		ID:        session.ID,
		ThreadID:  session.ThreadID,
		Locale:    session.Locale,
		Title:     session.Title,
		UpdatedAt: session.UpdatedAt,
		ExpiresAt: session.ExpiresAt,
	}
	return portfolioChatSessionResponse{
		Session:  &sessionDTO,
		Messages: items,
	}
}

func portfolioEmptySessionResponse() portfolioChatSessionResponse {
	return portfolioChatSessionResponse{Messages: []portfolioChatMessageDTO{}}
}

func portfolioVisitorHash(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext) (string, bool) {
	visitorID := ""
	if cookie, err := r.Cookie(portfolioVisitorCookieName); err == nil {
		visitorID = strings.TrimSpace(cookie.Value)
	}
	if visitorID == "" {
		var err error
		visitorID, err = newPortfolioVisitorID()
		if err != nil {
			return "", false
		}
		http.SetCookie(w, &http.Cookie{
			Name:     portfolioVisitorCookieName,
			Value:    visitorID,
			Path:     "/",
			HttpOnly: true,
			Secure:   shouldSecureCookie(svcCtx),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   portfolioVisitorCookieMaxAge,
		})
	}
	return hashPortfolioVisitorID(visitorID, portfolioChatVisitorSecret(svcCtx)), true
}

func newPortfolioVisitorID() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func hashPortfolioVisitorID(visitorID, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(visitorID))
	return hex.EncodeToString(mac.Sum(nil))
}

func portfolioChatVisitorSecret(svcCtx *svc.ServiceContext) string {
	if secret := strings.TrimSpace(svcCtx.Config.PortfolioChatVisitorSecret); secret != "" {
		return secret
	}
	// NewServiceContext rejects this fallback in production. It exists only for
	// explicit dev/test configurations so local setup remains frictionless.
	return "portfolio-chat-development-secret"
}

func portfolioChatExpiresAt(svcCtx *svc.ServiceContext) time.Time {
	ttlHours := svcCtx.Config.PortfolioChatSessionTTLHours
	if ttlHours <= 0 {
		ttlHours = defaultPortfolioChatTTLHours
	}
	return time.Now().UTC().Add(time.Duration(ttlHours) * time.Hour)
}

func portfolioChatMaxStoredMessages(svcCtx *svc.ServiceContext) int {
	maxMessages := svcCtx.Config.PortfolioChatMaxStoredMessages
	if maxMessages <= 0 {
		maxMessages = defaultPortfolioChatMaxMessages
	}
	if maxMessages > 500 {
		return 500
	}
	return maxMessages
}
