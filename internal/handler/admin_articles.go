package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"portfolio-backend/internal/logic"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

func isArticleStatusFilter(value string) bool {
	if value == "all" {
		return true
	}
	for _, s := range logic.ArticleStatuses {
		if s == value {
			return true
		}
	}
	return false
}

// AdminListArticlesHandler -> GET /api/admin/articles
func AdminListArticlesHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		q := r.URL.Query()
		limit := 20
		if v := q.Get("limit"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 100 {
				response.Error(w, http.StatusBadRequest, "Limit must be an integer between 1 and 100.",
					response.ErrorDetail{Field: "limit", Message: "Use a value between 1 and 100."})
				return
			}
			limit = n
		}
		page := 1
		if v := q.Get("page"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				response.Error(w, http.StatusBadRequest, "Page must be an integer greater than 0.",
					response.ErrorDetail{Field: "page", Message: "Use a value starting at 1."})
				return
			}
			page = n
		}

		status := q.Get("status")
		if status != "" && !isArticleStatusFilter(status) {
			response.Error(w, http.StatusBadRequest, "Status filter is invalid.",
				response.ErrorDetail{Field: "status", Message: "Use one of: all, " + strings.Join(logic.ArticleStatuses, ", ") + "."})
			return
		}
		if status == "" {
			status = "all"
		}

		result, err := svcCtx.Articles.List(r.Context(), model.ListArticlesInput{
			Limit: limit, Page: page, Status: status, Query: strings.TrimSpace(q.Get("query")),
		})
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load articles.")
			return
		}
		response.Ok(w, http.StatusOK, result)
	}
}

// AdminCreateArticleHandler -> POST /api/admin/articles
func AdminCreateArticleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		var payload logic.ArticlePayload
		if !decodeJSON(w, r, &payload) {
			return
		}

		input, details := logic.ValidateArticlePayload(payload)
		if input == nil {
			response.Error(w, http.StatusBadRequest, "Article payload is invalid.", details...)
			return
		}

		taken, err := svcCtx.Articles.IsSlugTaken(r.Context(), input.Slug, "")
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to create article.")
			return
		}
		if taken {
			response.Error(w, http.StatusConflict, "Slug is already in use.",
				response.ErrorDetail{Field: "slug", Message: "Choose a different slug."})
			return
		}

		article, err := svcCtx.Articles.Create(r.Context(), *input)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to create article.")
			return
		}
		svcCtx.ArticleCache.DeletePattern(r.Context(), articleCachePattern)
		response.Ok(w, http.StatusCreated, article)
	}
}

// AdminGetArticleHandler -> GET /api/admin/articles/{id}
func AdminGetArticleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := requireAdmin(w, r, svcCtx); !ok {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		article, err := svcCtx.Articles.FindByID(r.Context(), pathParam(r, "id"))
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load article.")
			return
		}
		if article == nil {
			response.Error(w, http.StatusNotFound, "Article was not found.")
			return
		}
		response.Ok(w, http.StatusOK, article)
	}
}

// AdminUpdateArticleHandler -> PATCH /api/admin/articles/{id}
func AdminUpdateArticleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		id := pathParam(r, "id")
		var payload logic.ArticlePayload
		if !decodeJSON(w, r, &payload) {
			return
		}

		input, details := logic.ValidateArticlePayload(payload)
		if input == nil {
			response.Error(w, http.StatusBadRequest, "Article payload is invalid.", details...)
			return
		}

		taken, err := svcCtx.Articles.IsSlugTaken(r.Context(), input.Slug, id)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to update article.")
			return
		}
		if taken {
			response.Error(w, http.StatusConflict, "Slug is already in use.",
				response.ErrorDetail{Field: "slug", Message: "Choose a different slug."})
			return
		}

		article, err := svcCtx.Articles.Update(r.Context(), id, *input)
		if errors.Is(err, model.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "Article was not found.")
			return
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to update article.")
			return
		}
		svcCtx.ArticleCache.DeletePattern(r.Context(), articleCachePattern)
		response.Ok(w, http.StatusOK, article)
	}
}

// AdminDeleteArticleHandler -> DELETE /api/admin/articles/{id}
func AdminDeleteArticleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, svcCtx)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		if !requireDatabase(w, svcCtx) {
			return
		}

		id := pathParam(r, "id")
		err := svcCtx.Articles.Delete(r.Context(), id)
		if errors.Is(err, model.ErrNotFound) {
			response.Error(w, http.StatusNotFound, "Article was not found.")
			return
		}
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to delete article.")
			return
		}
		svcCtx.ArticleCache.DeletePattern(r.Context(), articleCachePattern)
		response.Ok(w, http.StatusOK, map[string]any{"id": id})
	}
}
