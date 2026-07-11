package handler

import (
	"net/http"

	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.AddRoutes([]rest.Route{
		// Public
		{Method: http.MethodGet, Path: "/api/health", Handler: HealthHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/contact", Handler: ContactHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/articles", Handler: ArticlesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/articles/:slug", Handler: ArticleBySlugHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/studio/overview", Handler: StudioOverviewHandler(svcCtx)},

		// Admin Studio (viewer may list; admin/editor may mutate)
		{Method: http.MethodGet, Path: "/api/admin/studio/workflows", Handler: AdminListStudioWorkflowsHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/workflows", Handler: AdminCreateStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/studio/workflows/:id", Handler: AdminUpdateStudioWorkflowHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/executions", Handler: AdminListStudioExecutionsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/studio/audit-logs", Handler: AdminListStudioAuditsHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/pause", Handler: AdminStudioExecutionActionHandler(svcCtx, "pause")},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/retry", Handler: AdminStudioExecutionActionHandler(svcCtx, "retry")},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/cancel", Handler: AdminStudioExecutionActionHandler(svcCtx, "cancel")},
		{Method: http.MethodPost, Path: "/api/admin/studio/executions/:id/approve", Handler: AdminStudioExecutionActionHandler(svcCtx, "approve")},

		// Admin session
		{Method: http.MethodGet, Path: "/api/admin/session", Handler: SessionStatusHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/admin/session", Handler: SessionLoginHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/admin/session", Handler: SessionLogoutHandler(svcCtx)},

		// Admin contact inquiries
		{Method: http.MethodGet, Path: "/api/admin/contact-inquiries", Handler: AdminListInquiriesHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/admin/contact-inquiries/:id", Handler: AdminGetInquiryHandler(svcCtx)},
		{Method: http.MethodPatch, Path: "/api/admin/contact-inquiries/:id", Handler: AdminUpdateInquiryHandler(svcCtx)},

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

		// AI + jobs
		{Method: http.MethodPost, Path: "/api/ai/chat", Handler: AiChatHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/chat/stream", Handler: AiChatStreamHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/chat", Handler: PortfolioAssistantChatHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/chat/stream", Handler: PortfolioAssistantChatStreamHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/portfolio/assistant/sessions/current", Handler: PortfolioAssistantCurrentSessionHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/portfolio/assistant/sessions/latest", Handler: PortfolioAssistantLatestSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/portfolio/assistant/sessions", Handler: PortfolioAssistantNewSessionHandler(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/portfolio/assistant/sessions/:id", Handler: PortfolioAssistantDeleteSessionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/generate", Handler: AiGenerateHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/embed", Handler: AiEmbedHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ai/models", Handler: AiModelsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ai/running", Handler: AiRunningModelsHandler(svcCtx)},
		{Method: http.MethodGet, Path: "/api/ai/version", Handler: AiVersionHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/model/show", Handler: AiShowModelHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/ai/contact-summary", Handler: AiContactSummaryHandler(svcCtx)},
		{Method: http.MethodPost, Path: "/api/jobs/contact-follow-up", Handler: JobsContactFollowUpHandler(svcCtx)},
	})
}
