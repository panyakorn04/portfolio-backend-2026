package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

func TestValidateAIChatMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []model.OllamaChatMessage
		wantOK   bool
	}{
		{name: "valid", messages: []model.OllamaChatMessage{{Role: "user", Content: "hello"}}, wantOK: true},
		{name: "empty", messages: nil, wantOK: false},
		{name: "invalid role", messages: []model.OllamaChatMessage{{Role: "tool", Content: "hello"}}, wantOK: false},
		{name: "empty content", messages: []model.OllamaChatMessage{{Role: "user", Content: "   "}}, wantOK: false},
		{name: "too long", messages: []model.OllamaChatMessage{{Role: "user", Content: strings.Repeat("x", maxAIChatContentLength+1)}}, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, gotOK := validateAIChatMessages(tt.messages)
			if gotOK != tt.wantOK {
				t.Fatalf("validateAIChatMessages() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestResolveAIChatModelUsesWhitelist(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{OllamaAllowedModels: "panyakorn-local:latest, qwen2.5-coder:7b"},
		Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest"),
	}

	selected, _, ok := resolveAIChatModel(svcCtx, "qwen2.5-coder:7b")
	if !ok || selected != "qwen2.5-coder:7b" {
		t.Fatalf("selected = %q, ok = %v", selected, ok)
	}

	selected, _, ok = resolveAIChatModel(svcCtx, "")
	if !ok || selected != "panyakorn-local:latest" {
		t.Fatalf("default selected = %q, ok = %v", selected, ok)
	}

	if _, _, ok = resolveAIChatModel(svcCtx, "not-installed:latest"); ok {
		t.Fatal("unlisted model should be rejected")
	}
}

func TestAiChatHandlerForwardsSelectedModelToOllama(t *testing.T) {
	var got model.OllamaChatRequest
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen2.5-coder:7b","message":{"role":"assistant","content":"Coder ready"},"done":true}`))
	}))
	defer ollama.Close()

	svcCtx := &svc.ServiceContext{
		Config: config.Config{
			OllamaBaseURL:       ollama.URL,
			OllamaModel:         "panyakorn-local:latest",
			OllamaAllowedModels: "panyakorn-local:latest,qwen2.5-coder:7b",
		},
		Ollama: model.NewOllamaClient(ollama.URL, "panyakorn-local:latest"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"model":"qwen2.5-coder:7b","messages":[{"role":"user","content":"write code"}]}`))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Model != "qwen2.5-coder:7b" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestAiChatHandlerRejectsUnlistedModel(t *testing.T) {
	svcCtx := &svc.ServiceContext{
		Config: config.Config{OllamaAllowedModels: "panyakorn-local:latest"},
		Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest"),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"model":"not-installed:latest","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAiChatHandlerForwardsToOllama(t *testing.T) {
	var got model.OllamaChatRequest
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"พร้อมใช้งานครับ"},"done":true,"prompt_eval_count":5,"eval_count":7}`))
	}))
	defer ollama.Close()

	svcCtx := &svc.ServiceContext{
		Config: config.Config{OllamaBaseURL: ollama.URL, OllamaModel: "panyakorn-local:latest"},
		Ollama: model.NewOllamaClient(ollama.URL, "panyakorn-local:latest"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":" สวัสดี "}]}`))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Model != "panyakorn-local:latest" {
		t.Fatalf("model = %q", got.Model)
	}
	if got.Stream {
		t.Fatal("stream should be false")
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" || got.Messages[0].Content != "สวัสดี" {
		t.Fatalf("unexpected messages: %#v", got.Messages)
	}

	var body struct {
		Ok   bool `json:"ok"`
		Data struct {
			Model   string `json:"model"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			Done  bool `json:"done"`
			Usage struct {
				PromptEvalCount int `json:"prompt_eval_count"`
				EvalCount       int `json:"eval_count"`
			} `json:"usage"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Ok || body.Data.Message.Content != "พร้อมใช้งานครับ" || body.Data.Usage.EvalCount != 7 {
		t.Fatalf("unexpected response: %#v", body)
	}
}

func TestAiChatHandlerPinsAntiHallucinationGuardrailSkill(t *testing.T) {
	skillsDir := t.TempDir()
	writeHandlerTestSkill(t, skillsDir, "ai-console", "anti-hallucination-guardrails", "Guardrail skill: verify before claim.")
	writeHandlerTestSkill(t, skillsDir, "ai-console", "vps-ai-services", "VPS skill should not be auto-selected.")

	var got model.OllamaChatRequest
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"Guarded answer"},"done":true}`))
	}))
	defer ollama.Close()

	svcCtx := &svc.ServiceContext{
		Config:   config.Config{OllamaBaseURL: ollama.URL, OllamaModel: "panyakorn-local:latest", AISkillsDir: skillsDir},
		Ollama:   model.NewOllamaClient(ollama.URL, "panyakorn-local:latest"),
		AISkills: model.NewAISkillProfileStore(skillsDir),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"user","content":"ช่วยดู VPS docker deploy"}]}`))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, messages=%#v", len(got.Messages), got.Messages)
	}
	if got.Messages[0].Role != "system" || !strings.Contains(got.Messages[0].Content, "anti-hallucination-guardrails") || !strings.Contains(got.Messages[0].Content, "Guardrail skill") {
		t.Fatalf("missing pinned guardrail skill context: %#v", got.Messages[0])
	}
	if strings.Contains(got.Messages[0].Content, "VPS skill should not be auto-selected") {
		t.Fatalf("unexpected unrelated ai-console skill context: %#v", got.Messages[0])
	}
}

func TestPortfolioAssistantChatHandlerRequiresPersistedStream(t *testing.T) {
	svcCtx := &svc.ServiceContext{Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest")}
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat", strings.NewReader(`{"messages":[{"role":"user","content":"รับทำเว็บอะไรบ้าง"}]}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAiChatStreamHandlerForwardsOllamaStreamAsSSE(t *testing.T) {
	var got model.OllamaChatRequest
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"API "},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"OK"},"done":true,"prompt_eval_count":5,"eval_count":2}` + "\n"))
	}))
	defer ollama.Close()

	svcCtx := &svc.ServiceContext{
		Config: config.Config{OllamaBaseURL: ollama.URL, OllamaModel: "panyakorn-local:latest"},
		Ollama: model.NewOllamaClient(ollama.URL, "panyakorn-local:latest"),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat/stream", strings.NewReader(`{"threadId":"thread-test","runId":"run-test","messages":[{"role":"user","content":"ping"}]}`))
	rec := httptest.NewRecorder()

	AiChatStreamHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		t.Fatalf("content-type = %q", contentType)
	}
	if !got.Stream {
		t.Fatal("ollama stream should be true")
	}

	var eventTypes []string
	var loopStages []string
	var deltas []string
	scanner := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			t.Fatalf("decode SSE event %q: %v", line, err)
		}
		eventType := event["type"].(string)
		eventTypes = append(eventTypes, eventType)
		if eventType == "LOOP_STAGE" {
			loopStages = append(loopStages, event["stage"].(string))
		}
		if delta, ok := event["delta"].(string); ok {
			deltas = append(deltas, delta)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan SSE: %v", err)
	}

	joinedTypes := strings.Join(eventTypes, ",")
	for _, want := range []string{"RUN_STARTED", "LOOP_STAGE", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END", "RUN_FINISHED"} {
		if !strings.Contains(joinedTypes, want) {
			t.Fatalf("missing event %s in %v; body=%s", want, eventTypes, rec.Body.String())
		}
	}
	for _, want := range []string{"observing", "classifying", "planning", "retrieving_context", "drafting", "evaluating", "finalizing"} {
		if !strings.Contains(strings.Join(loopStages, ","), want) {
			t.Fatalf("missing loop stage %s in %v; body=%s", want, loopStages, rec.Body.String())
		}
	}
	if strings.Join(deltas, "") != "API OK" {
		t.Fatalf("deltas = %#v", deltas)
	}
}

func TestPortfolioAssistantChatStreamPersistsAtomicallyBeforeFinishing(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"API OK"},"done":true}`))
	}))
	defer ollama.Close()

	now := time.Now().UTC()
	rpcCalled := false
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/v1/PortfolioChatSession":
			_, _ = fmt.Fprintf(w, `[{"id":"session-1","visitorIdHash":"hash","threadId":"thread-1","locale":"th","status":"active","createdAt":%q,"updatedAt":%q,"lastSeenAt":%q,"expiresAt":%q}]`, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
		case r.Method == http.MethodGet && r.URL.Path == "/rest/v1/PortfolioChatMessage":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/claimPortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"claimed"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/rest/v1/rpc/completePortfolioChatRun":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["runId"] != "run-atomic" || body["assistantContent"] != "API OK" {
				t.Fatalf("persistence body = %#v", body)
			}
			rpcCalled = true
			_, _ = fmt.Fprintf(w, `[{"outcome":"inserted","userMessage":{"id":"user-1","sessionId":"session-1","role":"user","type":"chat","content":"hello","createdAt":%q},"assistantMessage":{"id":"assistant-1","sessionId":"session-1","role":"assistant","type":"chat","content":"API OK","createdAt":%q}}]`, now.Format(time.RFC3339), now.Format(time.RFC3339))
		default:
			t.Fatalf("unexpected persistence request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	svcCtx.Config.OllamaBaseURL = ollama.URL
	svcCtx.Config.OllamaModel = "panyakorn-local:latest"
	svcCtx.Ollama = model.NewOllamaClient(ollama.URL, "panyakorn-local:latest")
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat/stream", strings.NewReader(`{"sessionId":"session-1","runId":"run-atomic","message":{"role":"user","content":"hello"}}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatStreamHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !rpcCalled {
		t.Fatalf("status = %d, rpcCalled = %v, body = %s", rec.Code, rpcCalled, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"type":"RUN_ERROR"`) || !strings.Contains(rec.Body.String(), `"type":"RUN_FINISHED"`) {
		t.Fatalf("unexpected stream events: %s", rec.Body.String())
	}
}

func TestPortfolioAssistantChatStreamDoesNotDeliverContentAfterConcurrentHandoff(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"must stay hidden"},"done":true}`))
	}))
	defer ollama.Close()

	now := time.Now().UTC()
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/PortfolioChatSession":
			_, _ = fmt.Fprintf(w, `[{"id":"session-1","visitorIdHash":"hash","threadId":"thread-1","locale":"th","status":"active","createdAt":%q,"updatedAt":%q,"lastSeenAt":%q,"expiresAt":%q}]`, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
		case "/rest/v1/rpc/claimPortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"claimed"}]`))
		case "/rest/v1/PortfolioChatMessage":
			_, _ = w.Write([]byte(`[]`))
		case "/rest/v1/rpc/completePortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"state_conflict"}]`))
		case "/rest/v1/rpc/releasePortfolioChatRun":
			_, _ = w.Write([]byte(`true`))
		default:
			t.Fatalf("unexpected persistence request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	svcCtx.Config.OllamaBaseURL = ollama.URL
	svcCtx.Config.OllamaModel = "panyakorn-local:latest"
	svcCtx.Ollama = model.NewOllamaClient(ollama.URL, "panyakorn-local:latest")
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat/stream", strings.NewReader(`{"sessionId":"session-1","runId":"run-race","message":{"role":"user","content":"hello"}}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatStreamHandler(svcCtx).ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), "must stay hidden") || !strings.Contains(rec.Body.String(), "CHAT_STATE_CONFLICT") {
		t.Fatalf("AI content escaped after handoff: %s", rec.Body.String())
	}
}

func TestPortfolioAssistantChatStreamRejectsNonActiveSessions(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{name: "waiting for human", status: "pending_human"},
		{name: "human takeover", status: "human"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ollamaCalled := false
			ollama := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				ollamaCalled = true
			}))
			defer ollama.Close()

			now := time.Now().UTC()
			supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/rest/v1/PortfolioChatSession" || r.Method != http.MethodGet {
					t.Fatalf("unexpected persistence request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `[{"id":"session-1","visitorIdHash":"hash","threadId":"thread-1","locale":"th","status":%q,"createdAt":%q,"updatedAt":%q,"lastSeenAt":%q,"expiresAt":%q}]`, tt.status, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
			}))
			defer supabase.Close()

			svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
			svcCtx.Config.OllamaBaseURL = ollama.URL
			svcCtx.Config.OllamaModel = "panyakorn-local:latest"
			svcCtx.Ollama = model.NewOllamaClient(ollama.URL, "panyakorn-local:latest")

			req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat/stream", strings.NewReader(`{"sessionId":"session-1","message":{"role":"user","content":"hello"}}`))
			req.AddCookie(&http.Cookie{Name: portfolioVisitorCookieName, Value: "visitor-1"})
			rec := httptest.NewRecorder()

			PortfolioAssistantChatStreamHandler(svcCtx).ServeHTTP(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if ollamaCalled {
				t.Fatal("Ollama must not be called after human handoff")
			}
		})
	}
}

func TestPortfolioAssistantChatStreamRejectsLegacyMessagesShape(t *testing.T) {
	svcCtx := &svc.ServiceContext{Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest")}
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat/stream", strings.NewReader(`{"sessionId":"session-1","runId":"run-1","messages":[{"role":"user","content":"hello"}]}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatStreamHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestPortfolioAssistantChatStreamReplaysStoredExchangeWithoutOllama(t *testing.T) {
	ollamaCalled := false
	ollama := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ollamaCalled = true }))
	defer ollama.Close()

	now := time.Now().UTC()
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/PortfolioChatSession":
			_, _ = fmt.Fprintf(w, `[{"id":"session-1","visitorIdHash":"hash","threadId":"thread-1","locale":"th","status":"active","createdAt":%q,"updatedAt":%q,"lastSeenAt":%q,"expiresAt":%q}]`, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
		case "/rest/v1/rpc/claimPortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"replayed","assistantContent":"stored answer","modelName":"panyakorn-local:latest"}]`))
		case "/rest/v1/PortfolioChatMessage":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Fatalf("unexpected persistence request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	svcCtx.Config.OllamaBaseURL = ollama.URL
	svcCtx.Config.OllamaModel = "panyakorn-local:latest"
	svcCtx.Ollama = model.NewOllamaClient(ollama.URL, "panyakorn-local:latest")
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat/stream", strings.NewReader(`{"sessionId":"session-1","runId":"run-replay","message":{"role":"user","content":"hello"}}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatStreamHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "stored answer") || !strings.Contains(rec.Body.String(), `"type":"RUN_FINISHED"`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ollamaCalled {
		t.Fatal("Ollama must not be called when replaying a persisted runId")
	}
}

func TestPortfolioAssistantChatStreamRejectsConcurrentRunBeforeOllama(t *testing.T) {
	ollamaCalled := false
	ollama := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ollamaCalled = true }))
	defer ollama.Close()

	now := time.Now().UTC()
	supabase := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/v1/PortfolioChatSession":
			_, _ = fmt.Fprintf(w, `[{"id":"session-1","visitorIdHash":"hash","threadId":"thread-1","locale":"th","status":"active","createdAt":%q,"updatedAt":%q,"lastSeenAt":%q,"expiresAt":%q}]`, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
		case "/rest/v1/rpc/claimPortfolioChatRun":
			_, _ = w.Write([]byte(`[{"outcome":"in_progress"}]`))
		default:
			t.Fatalf("unexpected persistence request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer supabase.Close()

	svcCtx := newPortfolioChatSessionTestContext(supabase.URL)
	svcCtx.Ollama = model.NewOllamaClient(ollama.URL, "panyakorn-local:latest")
	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat/stream", strings.NewReader(`{"sessionId":"session-1","runId":"run-concurrent","message":{"role":"user","content":"hello"}}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatStreamHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || rec.Header().Get("Retry-After") != "3" {
		t.Fatalf("status = %d, retry-after = %q, body = %s", rec.Code, rec.Header().Get("Retry-After"), rec.Body.String())
	}
	if ollamaCalled {
		t.Fatal("Ollama must not be called for an already claimed run")
	}
}

func TestAiChatHandlerRejectsInvalidMessages(t *testing.T) {
	svcCtx := &svc.ServiceContext{Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest")}
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"tool","content":"no"}]}`))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAiChatHandlerRejectsOversizedBody(t *testing.T) {
	svcCtx := &svc.ServiceContext{Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest")}
	body := `{"messages":[{"role":"user","content":"` + strings.Repeat("x", maxAIChatBodyBytes) + `"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(body))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func writeHandlerTestSkill(t *testing.T, baseDir, profile, skill, body string) {
	t.Helper()
	dir := filepath.Join(baseDir, profile, skill)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + skill + "\ndescription: Test skill.\n---\n\n# " + skill + "\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
