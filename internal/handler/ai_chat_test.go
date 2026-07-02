package handler

import (
	"bufio"
	"encoding/json"
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

func TestPortfolioAssistantChatHandlerInjectsPortfolioSkillProfile(t *testing.T) {
	skillsDir := t.TempDir()
	profileDir := filepath.Join(skillsDir, "portfolio-site", "portfolio-profile")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("create skill profile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "SKILL.md"), []byte("Portfolio-only skill: answer about Panyakorn services and never mention VPS internals."), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	var got model.OllamaChatRequest
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","message":{"role":"assistant","content":"Portfolio answer"},"done":true}`))
	}))
	defer ollama.Close()

	svcCtx := &svc.ServiceContext{
		Config:   config.Config{OllamaBaseURL: ollama.URL, OllamaModel: "panyakorn-local:latest", AISkillsDir: skillsDir},
		Ollama:   model.NewOllamaClient(ollama.URL, "panyakorn-local:latest"),
		AISkills: model.NewAISkillProfileStore(skillsDir),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/portfolio/assistant/chat", strings.NewReader(`{"messages":[{"role":"user","content":"รับทำเว็บอะไรบ้าง"}]}`))
	rec := httptest.NewRecorder()

	PortfolioAssistantChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages len = %d, messages=%#v", len(got.Messages), got.Messages)
	}
	if got.Messages[0].Role != "system" || !strings.Contains(got.Messages[0].Content, `"portfolio-site"`) || !strings.Contains(got.Messages[0].Content, "Portfolio-only skill") {
		t.Fatalf("missing portfolio skill context: %#v", got.Messages[0])
	}
	if got.Messages[1].Role != "user" || got.Messages[1].Content != "รับทำเว็บอะไรบ้าง" {
		t.Fatalf("unexpected user message: %#v", got.Messages[1])
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
		eventTypes = append(eventTypes, event["type"].(string))
		if delta, ok := event["delta"].(string); ok {
			deltas = append(deltas, delta)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan SSE: %v", err)
	}

	joinedTypes := strings.Join(eventTypes, ",")
	for _, want := range []string{"RUN_STARTED", "TEXT_MESSAGE_START", "TEXT_MESSAGE_CONTENT", "TEXT_MESSAGE_END", "RUN_FINISHED"} {
		if !strings.Contains(joinedTypes, want) {
			t.Fatalf("missing event %s in %v; body=%s", want, eventTypes, rec.Body.String())
		}
	}
	if strings.Join(deltas, "") != "API OK" {
		t.Fatalf("deltas = %#v", deltas)
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

func TestAIChatRateLimiterAllowsWithinWindow(t *testing.T) {
	limiter := newAIChatRateLimiter(2, time.Minute)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if !limiter.allow("client", now) {
		t.Fatal("first request should be allowed")
	}
	if !limiter.allow("client", now.Add(time.Second)) {
		t.Fatal("second request should be allowed")
	}
	if limiter.allow("client", now.Add(2*time.Second)) {
		t.Fatal("third request in the same window should be rejected")
	}
	if !limiter.allow("client", now.Add(2*time.Minute)) {
		t.Fatal("request after the window should be allowed")
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
