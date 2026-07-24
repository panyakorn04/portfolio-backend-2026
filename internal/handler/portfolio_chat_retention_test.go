package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

func TestPortfolioChatRetentionRunnerDrainsBoundedBatches(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/v1/rpc/prunePortfolioChatRetention" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`[{"messagesDeleted":1,"sessionsDeleted":1}]`))
			return
		}
		_, _ = w.Write([]byte(`[{"messagesDeleted":0,"sessionsDeleted":0}]`))
	}))
	defer server.Close()

	rest := model.NewSupabaseREST(server.URL, "test-key")
	runner := NewPortfolioChatRetentionRunner(&svc.ServiceContext{
		Config:                config.Config{PortfolioChatMaxStoredMessages: 50},
		PortfolioChatSessions: model.NewPortfolioChatSessionModel(rest),
	})
	runner.run(context.Background())
	if got := calls.Load(); got != 2 {
		t.Fatalf("retention calls = %d, want 2", got)
	}
}
