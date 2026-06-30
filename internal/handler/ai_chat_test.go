package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestAiChatHandlerRejectsInvalidMessages(t *testing.T) {
	svcCtx := &svc.ServiceContext{Ollama: model.NewOllamaClient("http://127.0.0.1:1", "panyakorn-local:latest")}
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"messages":[{"role":"tool","content":"no"}]}`))
	rec := httptest.NewRecorder()

	AiChatHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
