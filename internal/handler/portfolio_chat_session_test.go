package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

func TestPortfolioAssistantNewSessionRejectsMissingTitle(t *testing.T) {
	svcCtx := newPortfolioChatSessionTestContext("http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/sessions", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantNewSessionHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Title is required") {
		t.Fatalf("expected title validation error, body = %s", rec.Body.String())
	}
}

func TestPortfolioAssistantNewSessionRejectsOversizedTitle(t *testing.T) {
	svcCtx := newPortfolioChatSessionTestContext("http://127.0.0.1:1")
	body := `{"title":"` + strings.Repeat("x", maxPortfolioChatTitleLength+1) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/sessions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	PortfolioAssistantNewSessionHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Title is too long") {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioChatMaxStoredMessagesRejectsValuesBelowDatabaseMinimum(t *testing.T) {
	svcCtx := &svc.ServiceContext{Config: config.Config{PortfolioChatMaxStoredMessages: 1}}
	if got := portfolioChatMaxStoredMessages(svcCtx); got != defaultPortfolioChatMaxMessages {
		t.Fatalf("max stored messages = %d, want %d", got, defaultPortfolioChatMaxMessages)
	}
}

func TestPortfolioAssistantNewSessionPersistsTitle(t *testing.T) {
	var got map[string]any
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/v1/PortfolioChatSession" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		now := time.Now().UTC().Format(time.RFC3339)
		_, _ = w.Write([]byte(`[{"id":"session-1","visitorIdHash":"` + got["visitorIdHash"].(string) + `","threadId":"` + got["threadId"].(string) + `","locale":"` + got["locale"].(string) + `","title":"` + got["title"].(string) + `","createdAt":"` + now + `","updatedAt":"` + now + `","lastSeenAt":"` + now + `","expiresAt":"` + got["expiresAt"].(string) + `"}]`))
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/sessions", strings.NewReader(`{"title":"  New portfolio chat  ","locale":"th"}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantNewSessionHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got["title"] != "New portfolio chat" {
		t.Fatalf("title sent to Supabase = %#v", got["title"])
	}
	if got["locale"] != "th" {
		t.Fatalf("locale sent to Supabase = %#v", got["locale"])
	}
	if !strings.Contains(rec.Body.String(), `"title":"New portfolio chat"`) {
		t.Fatalf("response missing title, body = %s", rec.Body.String())
	}
}

func TestPortfolioAssistantLatestSessionReturnsEmptyWhenNoneExists(t *testing.T) {
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/v1/PortfolioChatSession" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/assistant/sessions/latest?locale=th", nil)
	rec := httptest.NewRecorder()

	PortfolioAssistantLatestSessionHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"session":null`) || !strings.Contains(rec.Body.String(), `"messages":[]`) {
		t.Fatalf("expected empty latest session response, body = %s", rec.Body.String())
	}
}

func TestPortfolioAssistantLatestSessionReturnsSessionForChatStream(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/PortfolioChatSession":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected session method %s", r.Method)
			}
			_, _ = w.Write([]byte(`[{"id":"session-latest","visitorIdHash":"visitor-hash","threadId":"thread-latest","locale":"th","title":"Latest chat","createdAt":"` + now + `","updatedAt":"` + now + `","lastSeenAt":"` + now + `","expiresAt":"` + now + `"}]`))
		case "/rest/v1/PortfolioChatMessage":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected message method %s", r.Method)
			}
			_, _ = w.Write([]byte(`[{"id":"message-1","sessionId":"session-latest","role":"user","content":"สวัสดี","createdAt":"` + now + `","metadata":{}}]`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	req := httptest.NewRequest(http.MethodGet, "/api/portfolio/assistant/sessions/latest", nil)
	rec := httptest.NewRecorder()

	PortfolioAssistantLatestSessionHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Ok   bool `json:"ok"`
		Data struct {
			Session *struct {
				ID       string `json:"id"`
				ThreadID string `json:"threadId"`
				Title    string `json:"title"`
			} `json:"session"`
			Messages []struct {
				ID   string `json:"id"`
				Role string `json:"role"`
				Text string `json:"text"`
			} `json:"messages"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Ok || body.Data.Session == nil {
		t.Fatalf("expected latest session, body = %#v", body)
	}
	if body.Data.Session.ID != "session-latest" || body.Data.Session.ThreadID != "thread-latest" {
		t.Fatalf("unexpected session for chat stream: %#v", body.Data.Session)
	}
	if len(body.Data.Messages) != 1 || body.Data.Messages[0].Text != "สวัสดี" {
		t.Fatalf("unexpected messages: %#v", body.Data.Messages)
	}
}

func newPortfolioChatSessionTestContext(supabaseURL string) *svc.ServiceContext {
	supabase := model.NewSupabaseREST(supabaseURL, "test-key")
	return &svc.ServiceContext{
		Config:                config.Config{PortfolioChatVisitorSecret: "test-visitor-secret"},
		Supabase:              supabase,
		PortfolioChatSessions: model.NewPortfolioChatSessionModel(supabase),
		PortfolioChatMessages: model.NewPortfolioChatMessageModel(supabase),
		HasDatabse:            true,
	}
}
