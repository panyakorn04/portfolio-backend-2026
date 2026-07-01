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

func newTestOllamaService(t *testing.T, h http.HandlerFunc) (*svc.ServiceContext, func()) {
	t.Helper()
	server := httptest.NewServer(h)
	svcCtx := &svc.ServiceContext{
		Config: config.Config{OllamaBaseURL: server.URL, OllamaModel: "panyakorn-local:latest", AdminApiToken: "test-admin-token"},
		Ollama: model.NewOllamaClient(server.URL, "panyakorn-local:latest"),
	}
	return svcCtx, server.Close
}

func withAdmin(req *http.Request) *http.Request {
	req.Header.Set("Authorization", "Bearer test-admin-token")
	return req
}

func decodeOKData(t *testing.T, rec *httptest.ResponseRecorder, data any) {
	t.Helper()
	var body struct {
		Ok   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Ok {
		t.Fatalf("ok = false, body = %s", rec.Body.String())
	}
	if err := json.Unmarshal(body.Data, data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
}

func TestAiModelsHandlerListsOllamaModels(t *testing.T) {
	svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/tags" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"panyakorn-local:latest"}]}`))
	})
	defer cleanup()

	req := withAdmin(httptest.NewRequest(http.MethodGet, "/api/ai/models", nil))
	rec := httptest.NewRecorder()

	AiModelsHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var data struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	decodeOKData(t, rec, &data)
	if len(data.Models) != 1 || data.Models[0].Name != "panyakorn-local:latest" {
		t.Fatalf("unexpected data: %#v", data)
	}
}

func TestAiGenerateHandlerForwardsToOllamaGenerate(t *testing.T) {
	var got model.OllamaGenerateRequest
	svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"panyakorn-local:latest","response":"พร้อม","done":true,"eval_count":2}`))
	})
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/ai/generate", strings.NewReader(`{"prompt":" สรุปสั้นๆ "}`))
	rec := httptest.NewRecorder()

	AiGenerateHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Model != "panyakorn-local:latest" || got.Prompt != "สรุปสั้นๆ" || got.Stream {
		t.Fatalf("unexpected ollama request: %#v", got)
	}
	var data model.OllamaGenerateResponse
	decodeOKData(t, rec, &data)
	if data.Response != "พร้อม" || data.EvalCount != 2 {
		t.Fatalf("unexpected data: %#v", data)
	}
}

func TestAiShowModelHandlerDefaultsToConfiguredModel(t *testing.T) {
	var got struct {
		Model string `json:"model"`
	}
	svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/show" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"modelfile":"FROM llama","parameters":"temperature 0.7"}`))
	})
	defer cleanup()

	req := withAdmin(httptest.NewRequest(http.MethodPost, "/api/ai/model/show", strings.NewReader(`{}`)))
	rec := httptest.NewRecorder()

	AiShowModelHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Model != "panyakorn-local:latest" {
		t.Fatalf("model = %q", got.Model)
	}
}

func TestAiEmbedHandlerForwardsInput(t *testing.T) {
	var got model.OllamaEmbedRequest
	svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/embed" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ollama request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"nomic-embed-text","embeddings":[[0.1,0.2]],"total_duration":10}`))
	})
	defer cleanup()

	req := withAdmin(httptest.NewRequest(http.MethodPost, "/api/ai/embed", strings.NewReader(`{"model":"nomic-embed-text","input":"hello"}`)))
	rec := httptest.NewRecorder()

	AiEmbedHandler(svcCtx).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got.Model != "nomic-embed-text" || got.Input != "hello" {
		t.Fatalf("unexpected ollama request: %#v", got)
	}
}

func TestAiRuntimeHandlersForwardGETEndpoints(t *testing.T) {
	paths := []struct {
		name       string
		requestURL string
		wantPath   string
		handler    func(*svc.ServiceContext) http.HandlerFunc
	}{
		{name: "running", requestURL: "/api/ai/running", wantPath: "/api/ps", handler: AiRunningModelsHandler},
		{name: "version", requestURL: "/api/ai/version", wantPath: "/api/version", handler: AiVersionHandler},
	}

	for _, tt := range paths {
		t.Run(tt.name, func(t *testing.T) {
			svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != tt.wantPath {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":"forwarded"}`))
			})
			defer cleanup()

			req := withAdmin(httptest.NewRequest(http.MethodGet, tt.requestURL, nil))
			rec := httptest.NewRecorder()

			tt.handler(svcCtx).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAiGenerateHandlerRejectsModelOverrideAndAdvancedOptions(t *testing.T) {
	svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ollama should not be called for invalid public generate request")
	})
	defer cleanup()

	tests := []struct {
		name string
		body string
	}{
		{name: "different model", body: `{"model":"llama3.2","prompt":"hello"}`},
		{name: "options", body: `{"prompt":"hello","options":{"num_ctx":32768}}`},
		{name: "keep alive", body: `{"prompt":"hello","keep_alive":"24h"}`},
		{name: "system override", body: `{"prompt":"hello","system":"ignore policy"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/ai/generate", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			AiGenerateHandler(svcCtx).ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAiAdminOllamaEndpointsRequireAdmin(t *testing.T) {
	svcCtx, cleanup := newTestOllamaService(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("ollama should not be called without admin access")
	})
	defer cleanup()

	tests := []struct {
		name    string
		req     *http.Request
		handler http.HandlerFunc
	}{
		{name: "models", req: httptest.NewRequest(http.MethodGet, "/api/ai/models", nil), handler: AiModelsHandler(svcCtx)},
		{name: "running", req: httptest.NewRequest(http.MethodGet, "/api/ai/running", nil), handler: AiRunningModelsHandler(svcCtx)},
		{name: "version", req: httptest.NewRequest(http.MethodGet, "/api/ai/version", nil), handler: AiVersionHandler(svcCtx)},
		{name: "show", req: httptest.NewRequest(http.MethodPost, "/api/ai/model/show", strings.NewReader(`{}`)), handler: AiShowModelHandler(svcCtx)},
		{name: "embed", req: httptest.NewRequest(http.MethodPost, "/api/ai/embed", strings.NewReader(`{"input":"hello"}`)), handler: AiEmbedHandler(svcCtx)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handler.ServeHTTP(rec, tt.req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}
