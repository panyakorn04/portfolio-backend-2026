package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const (
	maxAIChatMessages      = 20
	maxAIChatContentLength = 4000
	maxAIChatBodyBytes     = 64 * 1024
	aiChatRequestsPerHour  = 30
)

var aiChatLimiter = newAIChatRateLimiter(aiChatRequestsPerHour, time.Hour)

type aiChatRequest struct {
	Messages []model.OllamaChatMessage `json:"messages"`
}

type aiChatStreamRequest struct {
	ThreadID string                    `json:"threadId"`
	RunID    string                    `json:"runId"`
	Messages []model.OllamaChatMessage `json:"messages"`
}

type aiChatResponse struct {
	Model   string                   `json:"model"`
	Message *model.OllamaChatMessage `json:"message,omitempty"`
	Done    bool                     `json:"done"`
	Usage   aiChatUsage              `json:"usage"`
}

type aiChatUsage struct {
	PromptEvalCount int `json:"prompt_eval_count,omitempty"`
	EvalCount       int `json:"eval_count,omitempty"`
}

func AiChatHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientKey := aiChatClientKey(r)
		if !aiChatLimiter.allow(clientKey, time.Now()) {
			response.Error(w, http.StatusTooManyRequests, "Too many AI chat requests. Please try again later.")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxAIChatBodyBytes)

		var body aiChatRequest
		if !decodeJSON(w, r, &body) {
			return
		}

		if errDetail, ok := validateAIChatMessages(body.Messages); !ok {
			response.Error(w, http.StatusBadRequest, "Invalid chat request.", errDetail)
			return
		}

		chat, err := svcCtx.Ollama.Chat(r.Context(), body.Messages)
		if err != nil {
			log.Printf("ai chat ollama error: %v", err)
			response.Error(w, http.StatusBadGateway, "Unable to get a response from the AI model.")
			return
		}

		response.Ok(w, http.StatusOK, aiChatResponse{
			Model:   chat.Model,
			Message: chat.Message,
			Done:    chat.Done,
			Usage: aiChatUsage{
				PromptEvalCount: chat.PromptEvalCount,
				EvalCount:       chat.EvalCount,
			},
		})
	}
}

func AiChatStreamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientKey := aiChatClientKey(r)
		if !aiChatLimiter.allow(clientKey, time.Now()) {
			response.Error(w, http.StatusTooManyRequests, "Too many AI chat requests. Please try again later.")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxAIChatBodyBytes)

		var body aiChatStreamRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		body.ThreadID = strings.TrimSpace(body.ThreadID)
		body.RunID = strings.TrimSpace(body.RunID)
		if body.ThreadID == "" {
			body.ThreadID = fmt.Sprintf("thread-%d", time.Now().UnixNano())
		}
		if body.RunID == "" {
			body.RunID = fmt.Sprintf("run-%d", time.Now().UnixNano())
		}

		if errDetail, ok := validateAIChatMessages(body.Messages); !ok {
			response.Error(w, http.StatusBadRequest, "Invalid chat request.", errDetail)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			response.Error(w, http.StatusInternalServerError, "Streaming is not supported by this server.")
			return
		}

		modelName := svcCtx.Ollama.Model()
		messageID := "assistant-" + body.RunID
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")

		writeAIStreamEvent(w, flusher, map[string]any{
			"type":      "RUN_STARTED",
			"timestamp": time.Now().UnixMilli(),
			"threadId":  body.ThreadID,
			"runId":     body.RunID,
			"model":     modelName,
		})
		writeAIStreamEvent(w, flusher, map[string]any{
			"type":      "TEXT_MESSAGE_START",
			"timestamp": time.Now().UnixMilli(),
			"messageId": messageID,
			"role":      "assistant",
			"model":     modelName,
		})

		var finalChunk model.OllamaChatResponse
		err := svcCtx.Ollama.ChatStream(r.Context(), body.Messages, func(chunk model.OllamaChatResponse) error {
			if chunk.Model != "" {
				modelName = chunk.Model
			}
			if chunk.Message != nil && chunk.Message.Content != "" {
				writeAIStreamEvent(w, flusher, map[string]any{
					"type":      "TEXT_MESSAGE_CONTENT",
					"timestamp": time.Now().UnixMilli(),
					"messageId": messageID,
					"delta":     chunk.Message.Content,
					"content":   chunk.Message.Content,
					"model":     modelName,
				})
			}
			if chunk.Done {
				finalChunk = chunk
			}
			return nil
		})
		if err != nil {
			log.Printf("ai chat stream ollama error: %v", err)
			writeAIStreamEvent(w, flusher, map[string]any{
				"type":      "RUN_ERROR",
				"timestamp": time.Now().UnixMilli(),
				"threadId":  body.ThreadID,
				"runId":     body.RunID,
				"model":     modelName,
				"message":   "Unable to stream a response from the AI model.",
				"code":      "OLLAMA_STREAM_ERROR",
			})
			return
		}

		writeAIStreamEvent(w, flusher, map[string]any{
			"type":      "TEXT_MESSAGE_END",
			"timestamp": time.Now().UnixMilli(),
			"messageId": messageID,
			"model":     modelName,
		})
		writeAIStreamEvent(w, flusher, map[string]any{
			"type":         "RUN_FINISHED",
			"timestamp":    time.Now().UnixMilli(),
			"threadId":     body.ThreadID,
			"runId":        body.RunID,
			"model":        modelName,
			"finishReason": "stop",
			"usage": map[string]int{
				"promptTokens":     finalChunk.PromptEvalCount,
				"completionTokens": finalChunk.EvalCount,
			},
		})
	}
}

func writeAIStreamEvent(w http.ResponseWriter, flusher http.Flusher, event map[string]any) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	flusher.Flush()
}

func validateAIChatMessages(messages []model.OllamaChatMessage) (response.ErrorDetail, bool) {
	if len(messages) == 0 || len(messages) > maxAIChatMessages {
		return response.ErrorDetail{Field: "messages", Message: "Provide 1-20 messages."}, false
	}

	for index, message := range messages {
		if message.Role != "system" && message.Role != "user" && message.Role != "assistant" {
			return response.ErrorDetail{Field: "messages.role", Message: "Role must be system, user, or assistant."}, false
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			return response.ErrorDetail{Field: "messages.content", Message: "Message content is required."}, false
		}
		if len(message.Content) > maxAIChatContentLength {
			return response.ErrorDetail{Field: "messages.content", Message: "Message content is too long."}, false
		}
		messages[index].Content = content
	}

	return response.ErrorDetail{}, true
}
