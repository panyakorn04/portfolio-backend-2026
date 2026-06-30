package handler

import (
	"net/http"
	"strings"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const (
	maxAIChatMessages      = 20
	maxAIChatContentLength = 4000
)

type aiChatRequest struct {
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
