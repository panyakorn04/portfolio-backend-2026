package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultOllamaBaseURL = "http://ollama:11434"
	DefaultOllamaModel   = "panyakorn-local:latest"
)

type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

type OllamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaChatRequest struct {
	Model    string              `json:"model"`
	Messages []OllamaChatMessage `json:"messages"`
	Stream   bool                `json:"stream"`
}

type OllamaChatResponse struct {
	Model     string             `json:"model"`
	CreatedAt string             `json:"created_at,omitempty"`
	Message   *OllamaChatMessage `json:"message,omitempty"`
	Done      bool               `json:"done"`

	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	EvalCount       int `json:"eval_count,omitempty"`
}

type OllamaGenerateRequest struct {
	Model     string         `json:"model"`
	Prompt    string         `json:"prompt"`
	Suffix    string         `json:"suffix,omitempty"`
	Images    []string       `json:"images,omitempty"`
	System    string         `json:"system,omitempty"`
	Template  string         `json:"template,omitempty"`
	Context   []int          `json:"context,omitempty"`
	Format    any            `json:"format,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Stream    bool           `json:"stream"`
	Raw       bool           `json:"raw,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
}

type OllamaGenerateResponse struct {
	Model     string `json:"model"`
	CreatedAt string `json:"created_at,omitempty"`
	Response  string `json:"response"`
	Done      bool   `json:"done"`
	Context   []int  `json:"context,omitempty"`

	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

type OllamaEmbedRequest struct {
	Model     string         `json:"model"`
	Input     any            `json:"input"`
	Truncate  *bool          `json:"truncate,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
}

type OllamaEmbedResponse struct {
	Model           string      `json:"model,omitempty"`
	Embeddings      [][]float64 `json:"embeddings,omitempty"`
	TotalDuration   int64       `json:"total_duration,omitempty"`
	LoadDuration    int64       `json:"load_duration,omitempty"`
	PromptEvalCount int         `json:"prompt_eval_count,omitempty"`
}

type OllamaShowRequest struct {
	Model   string `json:"model"`
	Verbose bool   `json:"verbose,omitempty"`
}

type OllamaModelListResponse struct {
	Models []OllamaModelInfo `json:"models"`
}

type OllamaModelInfo struct {
	Name       string                 `json:"name"`
	Model      string                 `json:"model,omitempty"`
	ModifiedAt string                 `json:"modified_at,omitempty"`
	Size       int64                  `json:"size,omitempty"`
	Digest     string                 `json:"digest,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
}

type OllamaRunningModelsResponse struct {
	Models []OllamaRunningModel `json:"models"`
}

type OllamaRunningModel struct {
	Name      string                 `json:"name"`
	Model     string                 `json:"model,omitempty"`
	Size      int64                  `json:"size,omitempty"`
	Digest    string                 `json:"digest,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	ExpiresAt string                 `json:"expires_at,omitempty"`
	SizeVRAM  int64                  `json:"size_vram,omitempty"`
}

type OllamaVersionResponse struct {
	Version string `json:"version"`
}

func NewOllamaClient(baseURL, model string) *OllamaClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultOllamaBaseURL
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultOllamaModel
	}

	return &OllamaClient{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *OllamaClient) Model() string {
	if c == nil || c.model == "" {
		return DefaultOllamaModel
	}
	return c.model
}

func (c *OllamaClient) Chat(ctx context.Context, messages []OllamaChatMessage) (*OllamaChatResponse, error) {
	if c == nil {
		return nil, fmt.Errorf("ollama client is not configured")
	}

	payload := OllamaChatRequest{
		Model:    c.Model(),
		Messages: messages,
		Stream:   false,
	}

	var out OllamaChatResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/chat", payload, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OllamaClient) Generate(ctx context.Context, req OllamaGenerateRequest) (*OllamaGenerateResponse, error) {
	if req.Model == "" {
		req.Model = c.Model()
	}
	req.Stream = false

	var out OllamaGenerateResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/generate", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OllamaClient) Embed(ctx context.Context, req OllamaEmbedRequest) (*OllamaEmbedResponse, error) {
	if req.Model == "" {
		req.Model = c.Model()
	}

	var out OllamaEmbedResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/embed", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OllamaClient) Show(ctx context.Context, req OllamaShowRequest) (map[string]any, error) {
	if req.Model == "" {
		req.Model = c.Model()
	}

	var out map[string]any
	if err := c.doJSON(ctx, http.MethodPost, "/api/show", req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *OllamaClient) ListModels(ctx context.Context) (*OllamaModelListResponse, error) {
	var out OllamaModelListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/tags", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OllamaClient) RunningModels(ctx context.Context) (*OllamaRunningModelsResponse, error) {
	var out OllamaRunningModelsResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/ps", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OllamaClient) Version(ctx context.Context) (*OllamaVersionResponse, error) {
	var out OllamaVersionResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/version", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *OllamaClient) doJSON(ctx context.Context, method, path string, in any, out any) error {
	if c == nil {
		return fmt.Errorf("ollama client is not configured")
	}

	var body io.Reader
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return err
	}
	return nil
}
