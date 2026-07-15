package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const (
	maxAIGeneratePromptLength = 8000
	maxAIAdapterBodyBytes     = 128 * 1024
	maxAIModelNameLength      = 160
)

type aiGenerateRequest struct {
	Model     string         `json:"model,omitempty"`
	Prompt    string         `json:"prompt"`
	Suffix    string         `json:"suffix,omitempty"`
	Images    []string       `json:"images,omitempty"`
	System    string         `json:"system,omitempty"`
	Template  string         `json:"template,omitempty"`
	Context   []int          `json:"context,omitempty"`
	Format    any            `json:"format,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
	Raw       bool           `json:"raw,omitempty"`
	KeepAlive string         `json:"keep_alive,omitempty"`
}

type aiShowModelRequest struct {
	Model   string `json:"model,omitempty"`
	Verbose bool   `json:"verbose,omitempty"`
}

type aiEmbedRequest struct {
	Model     string          `json:"model,omitempty"`
	Input     json.RawMessage `json:"input"`
	Truncate  *bool           `json:"truncate,omitempty"`
	Options   map[string]any  `json:"options,omitempty"`
	KeepAlive string          `json:"keep_alive,omitempty"`
}

func AiModelsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		models, err := svcCtx.Ollama.ListModels(r.Context())
		if err != nil {
			log.Printf("ai models ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to list Ollama models.")
			return
		}
		response.Ok(w, http.StatusOK, models)
	}
}

func AiRunningModelsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		models, err := svcCtx.Ollama.RunningModels(r.Context())
		if err != nil {
			log.Printf("ai running models ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to list running Ollama models.")
			return
		}
		response.Ok(w, http.StatusOK, models)
	}
}

func AiVersionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		version, err := svcCtx.Ollama.Version(r.Context())
		if err != nil {
			log.Printf("ai version ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to get Ollama version.")
			return
		}
		response.Ok(w, http.StatusOK, version)
	}
}

func AiGenerateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !allowAIInferenceRequest(w, r, svcCtx) {
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxAIAdapterBodyBytes)

		var body aiGenerateRequest
		if !decodeJSON(w, r, &body) {
			return
		}

		body.Model = strings.TrimSpace(body.Model)
		body.Prompt = strings.TrimSpace(body.Prompt)
		if errDetail, ok := validatePublicGenerateRequest(svcCtx, body); !ok {
			response.Error(w, http.StatusBadRequest, "Invalid generate request.", errDetail)
			return
		}

		result, err := svcCtx.Ollama.Generate(r.Context(), model.OllamaGenerateRequest{
			Prompt: body.Prompt,
			Format: body.Format,
		})
		if err != nil {
			log.Printf("ai generate ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to generate a response from the AI model.")
			return
		}
		response.Ok(w, http.StatusOK, result)
	}
}

func AiShowModelHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxAIAdapterBodyBytes)

		var body aiShowModelRequest
		if !decodeJSON(w, r, &body) {
			return
		}

		body.Model = strings.TrimSpace(body.Model)
		if errDetail, ok := validateOptionalAIModel(body.Model); !ok {
			response.Error(w, http.StatusBadRequest, "Invalid model request.", errDetail)
			return
		}

		result, err := svcCtx.Ollama.Show(r.Context(), model.OllamaShowRequest{Model: body.Model, Verbose: body.Verbose})
		if err != nil {
			log.Printf("ai show model ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to show Ollama model details.")
			return
		}
		response.Ok(w, http.StatusOK, result)
	}
}

func AiEmbedHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}
		if !allowAIInferenceRequest(w, r, svcCtx) {
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxAIAdapterBodyBytes)

		var body aiEmbedRequest
		if !decodeJSON(w, r, &body) {
			return
		}

		body.Model = strings.TrimSpace(body.Model)
		if errDetail, ok := validateOptionalAIModel(body.Model); !ok {
			response.Error(w, http.StatusBadRequest, "Invalid embed request.", errDetail)
			return
		}
		input, errDetail, ok := decodeEmbedInput(body.Input)
		if !ok {
			response.Error(w, http.StatusBadRequest, "Invalid embed request.", errDetail)
			return
		}

		result, err := svcCtx.Ollama.Embed(r.Context(), model.OllamaEmbedRequest{
			Model:     body.Model,
			Input:     input,
			Truncate:  body.Truncate,
			Options:   body.Options,
			KeepAlive: body.KeepAlive,
		})
		if err != nil {
			log.Printf("ai embed ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to create embeddings from the AI model.")
			return
		}
		response.Ok(w, http.StatusOK, result)
	}
}

func validateOptionalAIModel(name string) (response.ErrorDetail, bool) {
	if len(name) > maxAIModelNameLength {
		return response.ErrorDetail{Field: "model", Message: "Model name is too long."}, false
	}
	if strings.ContainsAny(name, "\x00\n\r	") {
		return response.ErrorDetail{Field: "model", Message: "Model name contains invalid whitespace."}, false
	}
	return response.ErrorDetail{}, true
}

func validatePublicGenerateRequest(svcCtx *svc.ServiceContext, body aiGenerateRequest) (response.ErrorDetail, bool) {
	if errDetail, ok := validateOptionalAIModel(body.Model); !ok {
		return errDetail, false
	}
	if body.Model != "" && body.Model != svcCtx.Ollama.Model() {
		return response.ErrorDetail{Field: "model", Message: "Public generate requests must use the configured default model."}, false
	}
	if body.Prompt == "" {
		return response.ErrorDetail{Field: "prompt", Message: "Prompt is required."}, false
	}
	if len(body.Prompt) > maxAIGeneratePromptLength {
		return response.ErrorDetail{Field: "prompt", Message: "Prompt is too long."}, false
	}
	if body.Suffix != "" || len(body.Images) > 0 || body.System != "" || body.Template != "" || len(body.Context) > 0 || len(body.Options) > 0 || body.Raw || body.KeepAlive != "" {
		return response.ErrorDetail{Field: "options", Message: "Public generate only supports prompt, optional default model, and format."}, false
	}
	return response.ErrorDetail{}, true
}

func allowAIInferenceRequest(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext) bool {
	clientKey := aiChatClientKey(r, svcCtx != nil && svcCtx.Config.TrustProxy)
	if !aiChatLimiter.allow(clientKey, time.Now()) {
		response.Error(w, http.StatusTooManyRequests, "Too many AI requests. Please try again later.")
		return false
	}
	return true
}

func decodeEmbedInput(raw json.RawMessage) (any, response.ErrorDetail, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, response.ErrorDetail{Field: "input", Message: "Input is required."}, false
	}

	var input any
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, response.ErrorDetail{Field: "input", Message: "Input must be valid JSON."}, false
	}

	switch value := input.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, response.ErrorDetail{Field: "input", Message: "Input is required."}, false
		}
	case []any:
		if len(value) == 0 {
			return nil, response.ErrorDetail{Field: "input", Message: "Input array cannot be empty."}, false
		}
		for _, item := range value {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, response.ErrorDetail{Field: "input", Message: "Input array must contain non-empty strings."}, false
			}
		}
	default:
		return nil, response.ErrorDetail{Field: "input", Message: "Input must be a string or an array of strings."}, false
	}

	return input, response.ErrorDetail{}, true
}
