package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

type adminChatSessionDTO struct {
	ID         string    `json:"id"`
	ThreadID   string    `json:"threadId"`
	Locale     string    `json:"locale"`
	Title      *string   `json:"title"`
	Status     string    `json:"status"`
	MessageQty int       `json:"messageQty"`
	UpdatedAt  time.Time `json:"updatedAt"`
	CreatedAt  time.Time `json:"createdAt"`
}

type adminChatMessageDTO struct {
	ID        string         `json:"id"`
	Role      string         `json:"role"`
	Type      string         `json:"type"`
	Text      string         `json:"text"`
	CreatedAt time.Time      `json:"createdAt"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type adminChatSessionDetailDTO struct {
	ID        string                 `json:"id"`
	ThreadID  string                 `json:"threadId"`
	Locale    string                 `json:"locale"`
	Title     *string                `json:"title"`
	Status    string                 `json:"status"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
	Messages  []adminChatMessageDTO  `json:"messages"`
}

type adminChatReplyRequest struct {
	Message string `json:"message"`
}

type adminChatUpdateStatusRequest struct {
	Status string `json:"status"`
}

func AdminListChatSessionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDatabase(w, svcCtx) {
			return
		}
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
		limitStr := strings.TrimSpace(r.URL.Query().Get("limit"))
		offsetStr := strings.TrimSpace(r.URL.Query().Get("offset"))

		limit := 50
		offset := 0
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 200 {
			limit = n
		}
		if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
			offset = n
		}

		sessions, total, err := svcCtx.PortfolioChatSessions.ListAll(r.Context(), statusFilter, limit, offset)
		if err != nil {
			log.Printf("admin list chat sessions error: %v", err)
			response.Error(w, http.StatusInternalServerError, "Unable to list chat sessions.")
			return
		}

		items := make([]adminChatSessionDTO, 0, len(sessions))
		for _, s := range sessions {
			messages, err := svcCtx.PortfolioChatMessages.ListForSession(r.Context(), s.ID, 0)
			msgQty := 0
			if err == nil {
				msgQty = len(messages)
			}
			items = append(items, adminChatSessionDTO{
				ID:         s.ID,
				ThreadID:   s.ThreadID,
				Locale:     s.Locale,
				Title:      s.Title,
				Status:     s.Status,
				MessageQty: msgQty,
				UpdatedAt:  s.UpdatedAt,
				CreatedAt:  s.CreatedAt,
			})
		}

		response.Ok(w, http.StatusOK, map[string]any{
			"sessions": items,
			"total":    total,
		})
	}
}

func AdminGetChatSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDatabase(w, svcCtx) {
			return
		}
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		sessionID := strings.TrimSpace(pathParam(r, "id"))
		if sessionID == "" {
			response.Error(w, http.StatusBadRequest, "Session id is required.")
			return
		}

		session, err := svcCtx.PortfolioChatSessions.FindByID(r.Context(), sessionID)
		if err != nil {
			log.Printf("admin get chat session error: %v", err)
			response.Error(w, http.StatusInternalServerError, "Unable to load chat session.")
			return
		}
		if session == nil {
			response.Error(w, http.StatusNotFound, "Chat session not found.")
			return
		}

		messages, err := svcCtx.PortfolioChatMessages.ListForSession(r.Context(), sessionID, 0)
		if err != nil {
			log.Printf("admin get chat messages error: %v", err)
			response.Error(w, http.StatusInternalServerError, "Unable to load chat messages.")
			return
		}

		msgDTOs := make([]adminChatMessageDTO, 0, len(messages))
		for _, m := range messages {
			msgDTOs = append(msgDTOs, adminChatMessageDTO{
				ID:        m.ID,
				Role:      m.Role,
				Type:      m.Type,
				Text:      m.Content,
				CreatedAt: m.CreatedAt,
				Metadata:  m.Metadata,
			})
		}

		response.Ok(w, http.StatusOK, adminChatSessionDetailDTO{
			ID:        session.ID,
			ThreadID:  session.ThreadID,
			Locale:    session.Locale,
			Title:     session.Title,
			Status:    session.Status,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.UpdatedAt,
			Messages:  msgDTOs,
		})
	}
}

func AdminReplyChatSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDatabase(w, svcCtx) {
			return
		}
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}

		sessionID := strings.TrimSpace(pathParam(r, "id"))
		if sessionID == "" {
			response.Error(w, http.StatusBadRequest, "Session id is required.")
			return
		}

		var payload adminChatReplyRequest
		if !decodeJSON(w, r, &payload) {
			return
		}
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			response.Error(w, http.StatusBadRequest, "Message is required.")
			return
		}

		session, err := svcCtx.PortfolioChatSessions.FindByID(r.Context(), sessionID)
		if err != nil {
			log.Printf("admin reply find session error: %v", err)
			response.Error(w, http.StatusInternalServerError, "Unable to load chat session.")
			return
		}
		if session == nil {
			response.Error(w, http.StatusNotFound, "Chat session not found.")
			return
		}

		meta := map[string]any{"source": "admin"}
		if access.User != nil && access.User.Name != nil {
			meta["adminName"] = *access.User.Name
		} else if access.User != nil {
			meta["adminName"] = access.User.Email
		}

		if session.Status == "active" {
			if err := svcCtx.PortfolioChatSessions.UpdateStatus(r.Context(), sessionID, "human"); err != nil {
				log.Printf("admin reply update status error: %v", err)
			}
		}

		msg, err := svcCtx.PortfolioChatMessages.Append(r.Context(), sessionID, "assistant", "chat", message, meta)
		if err != nil {
			log.Printf("admin reply append message error: %v", err)
			response.Error(w, http.StatusInternalServerError, "Unable to send reply.")
			return
		}

		response.Ok(w, http.StatusCreated, adminChatMessageDTO{
			ID:        msg.ID,
			Role:      msg.Role,
			Type:      msg.Type,
			Text:      msg.Content,
			CreatedAt: msg.CreatedAt,
			Metadata:  msg.Metadata,
		})
	}
}

func AdminUpdateChatSessionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireDatabase(w, svcCtx) {
			return
		}
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}

		sessionID := strings.TrimSpace(pathParam(r, "id"))
		if sessionID == "" {
			response.Error(w, http.StatusBadRequest, "Session id is required.")
			return
		}

		var payload adminChatUpdateStatusRequest
		if !decodeJSON(w, r, &payload) {
			return
		}

		validStatuses := map[string]bool{"active": true, "pending_human": true, "human": true}
		if !validStatuses[payload.Status] {
			response.Error(w, http.StatusBadRequest, "Invalid status. Must be: active, pending_human, or human.")
			return
		}

		if err := svcCtx.PortfolioChatSessions.UpdateStatus(r.Context(), sessionID, payload.Status); err != nil {
			log.Printf("admin update chat session status error: %v", err)
			response.Error(w, http.StatusInternalServerError, "Unable to update session status.")
			return
		}

		response.Ok(w, http.StatusOK, map[string]any{"status": payload.Status})
	}
}
