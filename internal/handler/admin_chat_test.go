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

	"github.com/zeromicro/go-zero/rest/pathvar"
)

func TestAdminReplyUsesAtomicTakeoverAndReplyRPC(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	var calls []string
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost || r.URL.Path != "/rest/v1/rpc/takeoverAndReplyPortfolioChat" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		calls = append(calls, "takeover-and-reply")
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["sessionId"] != "session-1" || body["replyContent"] != "Hello" || body["replyMessageId"] != "admin-reply-operation-1" {
			t.Fatalf("rpc body = %#v", body)
		}
		_, _ = w.Write([]byte(`[{"outcome":"replied","status":"human","replyMessage":{"id":"message-1","sessionId":"session-1","role":"assistant","type":"chat","content":"Hello","createdAt":"` + now + `","metadata":{"source":"admin"}}}]`))
	}))
	defer supabase.Close()

	rest := model.NewSupabaseREST(supabase.URL, "test-key")
	svcCtx := &svc.ServiceContext{
		Config:                config.Config{AdminApiToken: "test-token"},
		HasDatabse:            true,
		PortfolioChatSessions: model.NewPortfolioChatSessionModel(rest),
		PortfolioChatMessages: model.NewPortfolioChatMessageModel(rest),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/chat/sessions/session-1/reply", strings.NewReader(`{"message":"Hello","operationId":"operation-1"}`))
	req = pathvar.WithVars(req, map[string]string{"id": "session-1"})
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	AdminReplyChatSessionHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Join(calls, ",") != "takeover-and-reply" {
		t.Fatalf("persistence calls = %v", calls)
	}
}
