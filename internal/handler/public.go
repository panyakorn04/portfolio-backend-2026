package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"portfolio-backend/internal/logic"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const (
	maxContactBodyBytes       int64 = 8 * 1024
	defaultPublicArticleLimit       = 20
	maxPublicArticleLimit           = 100
)

func HealthHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		type supabaseStatus struct {
			Configured bool `json:"configured"`
			Reachable  bool `json:"reachable"`
		}
		type redisStatus struct {
			Configured bool `json:"configured"`
			Reachable  bool `json:"reachable"`
		}

		supabase := supabaseStatus{Configured: svcCtx.HasDatabse, Reachable: svcCtx.HasDatabse}
		redisCache := redisStatus{Configured: svcCtx.ArticleCache.Enabled()}
		if redisCache.Configured {
			ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
			redisCache.Reachable = svcCtx.ArticleCache.Ping(ctx) == nil
			cancel()
		}

		status := "ok"
		if !supabase.Configured {
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
				"supabase":          supabase,
				"redisArticleCache": redisCache,
				"webhook":           svcCtx.Config.ContactWebhookURL != "",
				"adminApiToken":     svcCtx.Config.AdminApiToken != "",
				"internalApiToken":  svcCtx.Config.InternalApiToken != "",
				"aiProvider":        aiProvider,
			},
		})
	}
}

func ContactHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enforceContactRateLimit(w, r, svcCtx) {
			return
		}
		var body logic.ContactSubmission
		if !decodeJSONWithLimit(w, r, &body, maxContactBodyBytes) {
			return
		}

		submission, details := logic.ValidateContactSubmission(body)
		if len(details) > 0 {
			response.Error(w, http.StatusUnprocessableEntity, "Contact submission validation failed.", details...)
			return
		}

		if !svcCtx.HasDatabse {
			response.Error(w, http.StatusServiceUnavailable, "Contact service is not configured yet.",
				response.ErrorDetail{Field: "NEXT_PUBLIC_SUPABASE_URL", Message: "Add Supabase REST URL and key configuration before using the contact API."})
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

		limitValue := defaultPublicArticleLimit
		if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
			n, err := strconv.Atoi(limitParam)
			if err != nil || n < 1 || n > maxPublicArticleLimit {
				response.Error(w, http.StatusBadRequest, "Invalid limit.",
					response.ErrorDetail{Field: "limit", Message: "Limit must be between 1 and 100."})
				return
			}
			limitValue = n
		}
		limit := &limitValue

		cacheKey := articleListCacheKey(locale, limit)
		if body, ok := svcCtx.ArticleCache.Get(r.Context(), cacheKey); ok {
			writeCachedJSON(w, body)
			return
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

		var total int
		if svcCtx.SupabaseArticles != nil {
			total, err = svcCtx.SupabaseArticles.CountPublishedForLocale(r.Context(), locale)
		} else {
			total, err = svcCtx.Articles.CountPublishedForLocale(r.Context(), locale)
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to count articles.")
			return
		}

		payload := map[string]any{
			"locale": locale,
			"total":  total,
			"items":  items,
		}
		body, err := response.MarshalOk(payload)
		if err != nil {
			response.Ok(w, http.StatusOK, payload)
			return
		}
		if svcCtx.ArticleCache.Enabled() {
			svcCtx.ArticleCache.Set(r.Context(), cacheKey, body)
			writeFreshJSON(w, body)
			return
		}
		response.WriteJSONBytes(w, http.StatusOK, body)
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
		cacheKey := articleDetailCacheKey(locale, slug)
		if body, ok := svcCtx.ArticleCache.Get(r.Context(), cacheKey); ok {
			writeCachedJSON(w, body)
			return
		}

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

		body, err := response.MarshalOk(detail)
		if err != nil {
			response.Ok(w, http.StatusOK, detail)
			return
		}
		if svcCtx.ArticleCache.Enabled() {
			svcCtx.ArticleCache.Set(r.Context(), cacheKey, body)
			writeFreshJSON(w, body)
			return
		}
		response.WriteJSONBytes(w, http.StatusOK, body)
	}
}
