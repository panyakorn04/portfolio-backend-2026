package handler

import (
	"net/http"
	"os"
	"path/filepath"

	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func SwaggerDocHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(filepath.Join("swagger.json"))
		if err != nil {
			http.Error(w, "swagger.json not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if _, err := w.Write(data); err != nil {
			return
		}
	}
}

func SwaggerUIHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(filepath.Join("swagger.html"))
		if err != nil {
			http.Error(w, "swagger.html not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write(data); err != nil {
			return
		}
	}
}

func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.AddRoutes([]rest.Route{
		// Public
		{Method: http.MethodGet, Path: "/api/health", Handler: HealthHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ready", Handler: StudioReadinessHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/contact", Handler: ContactHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/articles", Handler: ArticlesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/articles/:slug", Handler: ArticleBySlugHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/studio/overview", Handler: StudioOverviewHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/studio/executions/:id/stages", Handler: StudioExecutionStagesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/studio/executions/:id/events", Handler: StudioExecutionEventsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/studio/webhooks/:id/:nodeId", Handler: StudioWebhookHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/studio/webhooks/:id/:nodeId", Handler: StudioWebhookHandler(svcCtx)},

		// Admin Studio (viewer may list; admin/editor may mutate)
		{Method: http.MethodGet, Path: "/api/admin/studio/workflows", Handler: AdminListStudioWorkflowsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/workflows/:id", Handler: AdminGetStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/studio/workflows/:id", Handler: AdminDeleteStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/workflows/:id/executions", Handler: AdminEnqueueStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/workflows/:id/nodes/:nodeId/execute-previous", Handler: AdminExecuteStudioPreviousNodesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/workflows/:id/nodes/:nodeId/webhook-url", Handler: AdminStudioWebhookURLHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/workflows/:id/nodes/:nodeId/execute", Handler: AdminExecuteStudioTriggerHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/workflows/:id/nodes/:nodeId/http-request", Handler: AdminExecuteStudioHttpRequestHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/http-request/import-curl", Handler: AdminImportStudioCurlHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/credentials", Handler: AdminListStudioCredentialsHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/credentials", Handler: AdminCreateStudioCredentialHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/studio/credentials/:id", Handler: AdminUpdateStudioCredentialHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/studio/credentials/:id", Handler: AdminDeleteStudioCredentialHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/credentials/:id/test", Handler: AdminTestStudioCredentialHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/workflows", Handler: AdminCreateStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/studio/workflows/:id", Handler: AdminUpdateStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/executions", Handler: AdminListStudioExecutionsHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions", Handler: AdminEnqueueStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/executions/:id", Handler: AdminGetStudioExecutionHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/audit-logs", Handler: AdminListStudioAuditsHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/pause", Handler: AdminUnsupportedStudioExecutionActionHandler(svcCtx, "pause")},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/retry", Handler: AdminRetryStudioGraphExecutionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/cancel", Handler: AdminCancelStudioGraphExecutionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/approve", Handler: AdminUnsupportedStudioExecutionActionHandler(svcCtx, "approve")},

		// Admin session
		{Method: http.MethodGet, Path: "/api/admin/session", Handler: SessionStatusHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/session", Handler: SessionLoginHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/session", Handler: SessionLogoutHandler(svcCtx)},

		// Admin contact inquiries
		{Method: http.MethodGet, Path: "/api/admin/contact-inquiries", Handler: AdminListInquiriesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/contact-inquiries/:id", Handler: AdminGetInquiryHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/contact-inquiries/:id", Handler: AdminUpdateInquiryHandler(svcCtx)},

		// Admin chat sessions
		{Method: http.MethodGet, Path: "/api/admin/chat/sessions", Handler: AdminListChatSessionsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/chat/sessions/:id", Handler: AdminGetChatSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/chat/sessions/:id/reply", Handler: AdminReplyChatSessionHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/chat/sessions/:id", Handler: AdminUpdateChatSessionHandler(svcCtx)},

		// Admin sessions management
		{Method: http.MethodGet, Path: "/api/admin/sessions", Handler: AdminListSessionsHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/sessions", Handler: AdminLogoutEverywhereHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/sessions/:id", Handler: AdminRevokeSessionHandler(svcCtx)},

		// Admin users
		{Method: http.MethodGet, Path: "/api/admin/users", Handler: AdminListUsersHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/users/:id", Handler: AdminUpdateUserRoleHandler(svcCtx)},

		// Admin articles
		{Method: http.MethodGet, Path: "/api/admin/articles", Handler: AdminListArticlesHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/articles", Handler: AdminCreateArticleHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/articles/:id", Handler: AdminGetArticleHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/articles/:id", Handler: AdminUpdateArticleHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/articles/:id", Handler: AdminDeleteArticleHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/article-images", Handler: AdminUploadArticleImageHandler(svcCtx)},

		// AI + jobs
		{Method: http.MethodPost, Path: "/api/ai/chat", Handler: AiChatHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/chat/stream", Handler: AiChatStreamHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/chat", Handler: PortfolioAssistantChatHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/chat/stream", Handler: PortfolioAssistantChatStreamHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/portfolio/assistant/sessions/current", Handler: PortfolioAssistantCurrentSessionHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/portfolio/assistant/sessions/latest", Handler: PortfolioAssistantLatestSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/sessions", Handler: PortfolioAssistantNewSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/sessions/:id/request-human", Handler: PortfolioAssistantRequestHumanHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/portfolio/assistant/sessions/:id", Handler: PortfolioAssistantDeleteSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/generate", Handler: AiGenerateHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/embed", Handler: AiEmbedHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ai/models", Handler: AiModelsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ai/running", Handler: AiRunningModelsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ai/version", Handler: AiVersionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/model/show", Handler: AiShowModelHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/contact-summary", Handler: AiContactSummaryHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/jobs/contact-follow-up", Handler: JobsContactFollowUpHandler(svcCtx)},

		// Swagger
		{Method: http.MethodGet, Path: "/swagger/doc.json", Handler: SwaggerDocHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/swagger", Handler: SwaggerUIHandler(svcCtx)},
	})
}
