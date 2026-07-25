package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPortfolioChatTransactionalRPCContracts(t *testing.T) {
	t.Parallel()

	calls := map[string]map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		calls[r.URL.Path] = body
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/rpc/claimPortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"claimed"}]`))
		case "/rest/v1/rpc/completePortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"inserted","userMessage":{"id":"user-message","sessionId":"session-1","role":"user","type":"chat","content":"hello","createdAt":"2026-07-23T15:00:00Z"},"assistantMessage":{"id":"assistant-message","sessionId":"session-1","role":"assistant","type":"chat","content":"hi","createdAt":"2026-07-23T15:00:00Z"}}]`))
		case "/rest/v1/rpc/releasePortfolioChatRun":
			_, _ = w.Write([]byte(`true`))
		case "/rest/v1/rpc/requestPortfolioChatHuman":
			_, _ = w.Write([]byte(`[{"outcome":"updated","status":"pending_human","eventMessage":{"id":"event-message","sessionId":"session-1","role":"system","type":"request_human","content":"Visitor requested a human response.","createdAt":"2026-07-23T15:00:00Z"}}]`))
		case "/rest/v1/rpc/takeoverAndReplyPortfolioChat":
			_, _ = w.Write([]byte(`[{"outcome":"replied","status":"human","replyMessage":{"id":"reply-message","sessionId":"session-1","role":"assistant","type":"chat","content":"Admin reply","createdAt":"2026-07-23T15:00:00Z"}}]`))
		case "/rest/v1/rpc/prunePortfolioChatRetention":
			_, _ = w.Write([]byte(`[{"messagesDeleted":3,"sessionsDeleted":2}]`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	rest := NewSupabaseREST(server.URL, "test-key")
	sessions := NewPortfolioChatSessionModel(rest)
	messages := NewPortfolioChatMessageModel(rest)
	occurredAt := time.Date(2026, 7, 23, 15, 0, 0, 0, time.UTC)

	claim, err := messages.ClaimRun(context.Background(), PortfolioChatRunClaimInput{
		SessionID: "session-1", VisitorIDHash: "visitor-hash", RunID: "run-1", UserContent: "hello",
		LeaseOwner: "lease-1", LeaseUntil: occurredAt.Add(10 * time.Minute), OccurredAt: occurredAt,
	})
	if err != nil || claim.Outcome != PortfolioChatOutcomeClaimed {
		t.Fatalf("claim = %#v, err = %v", claim, err)
	}

	exchange, err := messages.CompleteRun(context.Background(), PortfolioChatExchangeInput{
		SessionID: "session-1", VisitorIDHash: "visitor-hash", RunID: "run-1",
		LeaseOwner:    "lease-1",
		UserMessageID: "user-message", AssistantMessageID: "assistant-message",
		UserContent: "hello", AssistantContent: "hi", ModelName: "model-1",
		ExpiresAt: occurredAt.Add(time.Hour), MaxMessages: 50, OccurredAt: occurredAt,
	})
	if err != nil || exchange.Outcome != PortfolioChatOutcomeInserted || exchange.AssistantMessage == nil {
		t.Fatalf("exchange = %#v, err = %v", exchange, err)
	}
	if got := calls["/rest/v1/rpc/completePortfolioChatRun"]["runId"]; got != "run-1" {
		t.Fatalf("persist runId = %#v", got)
	}
	if err := messages.ReleaseRun(context.Background(), "session-1", "visitor-hash", "run-1", "lease-1"); err != nil {
		t.Fatalf("release run: %v", err)
	}

	handoff, err := sessions.RequestHuman(context.Background(), PortfolioChatRequestHumanInput{
		SessionID: "session-1", VisitorIDHash: "visitor-hash", EventMessageID: "event-message",
		MaxMessages: 50, OccurredAt: occurredAt,
	})
	if err != nil || handoff.Outcome != PortfolioChatOutcomeUpdated || handoff.EventMessage == nil {
		t.Fatalf("handoff = %#v, err = %v", handoff, err)
	}

	reply, err := sessions.TakeoverAndReply(context.Background(), PortfolioChatTakeoverReplyInput{
		SessionID: "session-1", TakeoverMessageID: "takeover-message", ReplyMessageID: "reply-message",
		ReplyContent: "Admin reply", AdminName: "Admin", MaxMessages: 50, OccurredAt: occurredAt,
	})
	if err != nil || reply.Outcome != PortfolioChatOutcomeReplied || reply.ReplyMessage == nil {
		t.Fatalf("reply = %#v, err = %v", reply, err)
	}

	retention, err := sessions.PruneRetention(context.Background(), 50, occurredAt, 100)
	if err != nil || retention.MessagesDeleted != 3 || retention.SessionsDeleted != 2 {
		t.Fatalf("retention = %#v, err = %v", retention, err)
	}
	if got := calls["/rest/v1/rpc/prunePortfolioChatRetention"]["batchSize"]; got != float64(100) {
		t.Fatalf("retention batchSize = %#v", got)
	}
}
