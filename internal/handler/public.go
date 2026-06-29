package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"portfolio-backend/internal/logic"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

func HealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type dbStatus struct {
			Configured bool    `json:"configured"`
			Reachable  bool    `json:"reachable"`
			Error      *string `json:"error"`
		}

		db := dbStatus{Configured: svcCtx.HasDatabse}
		if svcCtx.HasDatabse {
			if err := svcCtx.DB.PingContext(r.Context()); err != nil {
				msg := err.Error()
				db.Error = &msg
			} else {
				db.Reachable = true
			}
		}

		status := "ok"
		if db.Configured && !db.Reachable {
			status = "degraded"
		}

		aiProvider := svcCtx.Config.AiProvider
		if aiProvider == "" {
			aiProvider = "stub"
		}

		response.Ok(w, http.StatusOK, map[string]any{
			"service":   "portfolio-api",
			"status":    status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"capabilities": map[string]any{
				"database":         db,
				"webhook":          svcCtx.Config.ContactWebhookURL != "",
				"adminApiToken":    svcCtx.Config.AdminApiToken != "",
				"internalApiToken": svcCtx.Config.InternalApiToken != "",
				"aiProvider":       aiProvider,
			},
		})
	}
}

func ContactHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body logic.ContactSubmission
		if !decodeJSON(w, r, &body) {
			return
		}

		submission, details := logic.ValidateContactSubmission(body)
		if len(details) > 0 {
			response.Error(w, http.StatusUnprocessableEntity, "Contact submission validation failed.", details...)
			return
		}

		if !svcCtx.HasDatabse {
			response.Error(w, http.StatusServiceUnavailable, "Contact service is not configured yet.",
				response.ErrorDetail{Field: "DATABASE_URL", Message: "Add a PostgreSQL connection string before using the contact API."})
			return
		}

		result, err := logic.PersistAndDeliver(r.Context(), svcCtx, submission)
		if err != nil {
			response.Error(w, http.StatusBadGateway, "Unable to save contact submission right now.")
			return
		}

		response.Ok(w, http.StatusCreated, map[string]any{
			"message":      "Contact submission accepted.",
			"inquiryId":    result.InquiryID,
			"deliveryMode": result.DeliveryMode,
			"submittedAt":  result.SubmittedAt,
		})
	}
}

func parsePublicLocale(value string) (string, bool) {
	if value == "" {
		value = "en"
	}
	for _, l := range logic.ArticleLocales {
		if l == value {
			return value, true
		}
	}
	return "", false
}

func ArticlesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		locale, ok := parsePublicLocale(r.URL.Query().Get("lang"))
		if !ok {
			response.Error(w, http.StatusBadRequest, "Unsupported locale.",
				response.ErrorDetail{Field: "lang", Message: "Use `en` or `th`."})
			return
		}

		var limit *int
		if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
			n, err := strconv.Atoi(limitParam)
			if err != nil || n < 1 {
				response.Error(w, http.StatusBadRequest, "Invalid limit.",
					response.ErrorDetail{Field: "limit", Message: "Limit must be a positive integer."})
				return
			}
			limit = &n
		}

		var articles []model.Article
		var err error
		if svcCtx.SupabaseArticles != nil {
			articles, err = svcCtx.SupabaseArticles.ListPublished(r.Context(), limit)
		} else {
			if !requireDatabase(w, svcCtx) {
				return
			}
			articles, err = svcCtx.Articles.ListPublished(r.Context(), limit)
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load articles.")
			return
		}

		items := make([]logic.ArticleListItem, 0, len(articles))
		for i := range articles {
			if item := logic.ToListItem(&articles[i], locale); item != nil {
				items = append(items, *item)
			}
		}

		total := len(items)
		if limit != nil {
			var all []model.Article
			if svcCtx.SupabaseArticles != nil {
				all, err = svcCtx.SupabaseArticles.ListPublished(r.Context(), nil)
			} else {
				all, err = svcCtx.Articles.ListPublished(r.Context(), nil)
			}
			if err == nil {
				total = 0
				for i := range all {
					if logic.ToListItem(&all[i], locale) != nil {
						total++
					}
				}
			}
		}

		response.Ok(w, http.StatusOK, map[string]any{
			"locale": locale,
			"total":  total,
			"items":  items,
		})
	}
}

func ArticleBySlugHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		locale, ok := parsePublicLocale(r.URL.Query().Get("lang"))
		if !ok {
			response.Error(w, http.StatusBadRequest, "Unsupported locale.",
				response.ErrorDetail{Field: "lang", Message: "Use `en` or `th`."})
			return
		}

		slug := strings.TrimSpace(pathParam(r, "slug"))
		var article *model.Article
		var err error
		if svcCtx.SupabaseArticles != nil {
			article, err = svcCtx.SupabaseArticles.FindPublishedBySlug(r.Context(), slug)
		} else {
			if !requireDatabase(w, svcCtx) {
				return
			}
			article, err = svcCtx.Articles.FindPublishedBySlug(r.Context(), slug)
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load article.")
			return
		}
		if article == nil {
			response.Error(w, http.StatusNotFound, "Article was not found.")
			return
		}

		detail := logic.ToDetail(article, locale)
		if detail == nil {
			response.Error(w, http.StatusNotFound, "Article was not found.")
			return
		}

		response.Ok(w, http.StatusOK, detail)
	}
}
