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

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out OllamaChatResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
