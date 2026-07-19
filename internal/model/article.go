package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type ArticleModel struct{ api *SupabaseREST }

func NewArticleModel(api *SupabaseREST) *ArticleModel { return &ArticleModel{api: api} }

type articleRow struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Category    string  `json:"category"`
	Status      string  `json:"status"`
	PublishedAt *string `json:"publishedAt"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

type articleTranslationRow struct {
	ID          string          `json:"id"`
	ArticleID   string          `json:"articleId"`
	Locale      string          `json:"locale"`
	Title       string          `json:"title"`
	Summary     string          `json:"summary"`
	Lead        string          `json:"lead"`
	ReadingTime string          `json:"readingTime"`
	Content     string          `json:"content"`
	Sections    json.RawMessage `json:"sections"`
}

func convertArticleRow(row articleRow) Article {
	return Article{ID: row.ID, Slug: row.Slug, Category: row.Category, Status: row.Status,
		PublishedAt: timePtrFromString(row.PublishedAt), CreatedAt: timeFromString(row.CreatedAt), UpdatedAt: timeFromString(row.UpdatedAt)}
}

func convertTranslationRow(row articleTranslationRow) (ArticleTranslation, error) {
	sections := []ArticleSection{}
	if len(row.Sections) > 0 && string(row.Sections) != "null" {
		if err := json.Unmarshal(row.Sections, &sections); err != nil {
			return ArticleTranslation{}, err
		}
	}
	return ArticleTranslation{
		ID:          row.ID,
		Locale:      row.Locale,
		Title:       row.Title,
		Summary:     row.Summary,
		Lead:        row.Lead,
		ReadingTime: row.ReadingTime,
		Content:     row.Content,
		Sections:    sections,
	}, nil
}

func (m *ArticleModel) loadTranslations(ctx context.Context, articles []Article) error {
	if len(articles) == 0 {
		return nil
	}
	ids := make([]string, 0, len(articles))
	idx := make(map[string]int, len(articles))
	for i := range articles {
		ids = append(ids, articles[i].ID)
		idx[articles[i].ID] = i
	}
	values := url.Values{}
	values.Set("select", "*")
	values.Set("articleId", "in.("+strings.Join(ids, ",")+")")
	values.Set("order", "locale.asc")
	var rows []articleTranslationRow
	if _, err := m.api.request(ctx, http.MethodGet, "ArticleTranslation", values, nil, "", &rows); err != nil {
		return err
	}
	for _, row := range rows {
		t, err := convertTranslationRow(row)
		if err != nil {
			return err
		}
		if i, ok := idx[row.ArticleID]; ok {
			articles[i].Translations = append(articles[i].Translations, t)
		}
	}
	for i := range articles {
		if articles[i].Translations == nil {
			articles[i].Translations = []ArticleTranslation{}
		}
	}
	return nil
}

func (m *ArticleModel) listWithValues(ctx context.Context, values url.Values) ([]Article, error) {
	values.Set("select", "*")
	var rows []articleRow
	if _, err := m.api.request(ctx, http.MethodGet, "Article", values, nil, "", &rows); err != nil {
		return nil, err
	}
	articles := make([]Article, 0, len(rows))
	for _, row := range rows {
		articles = append(articles, convertArticleRow(row))
	}
	if err := m.loadTranslations(ctx, articles); err != nil {
		return nil, err
	}
	return articles, nil
}

func (m *ArticleModel) ListPublished(ctx context.Context, limit *int) ([]Article, error) {
	values := url.Values{}
	values.Set("status", "eq.published")
	values.Set("order", "publishedAt.desc.nullslast,createdAt.desc")
	if limit != nil {
		values.Set("limit", fmt.Sprintf("%d", *limit))
	}
	return m.listWithValues(ctx, values)
}

func (m *ArticleModel) CountPublishedForLocale(ctx context.Context, locale string) (int, error) {
	values := url.Values{}
	values.Set("select", "id,Article!inner(id)")
	values.Set("locale", "eq."+locale)
	values.Set("Article.status", "eq.published")
	values.Set("limit", "1")
	var rows []struct {
		ID string `json:"id"`
	}
	resp, err := m.api.request(ctx, http.MethodGet, "ArticleTranslation", values, nil, "count=exact", &rows)
	if err != nil {
		return 0, err
	}
	return exactCount(resp, len(rows)), nil
}

func (m *ArticleModel) FindPublishedBySlug(ctx context.Context, slug string) (*Article, error) {
	values := url.Values{}
	values.Set("slug", "eq."+slug)
	values.Set("status", "eq.published")
	values.Set("limit", "1")
	items, err := m.listWithValues(ctx, values)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func (m *ArticleModel) FindByID(ctx context.Context, id string) (*Article, error) {
	values := url.Values{}
	values.Set("id", "eq."+id)
	values.Set("limit", "1")
	items, err := m.listWithValues(ctx, values)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

type ListArticlesInput struct {
	Limit  int
	Page   int
	Status string
	Query  string
}
type ListArticlesResult struct {
	Items      []Article `json:"items"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	Limit      int       `json:"limit"`
	TotalPages int       `json:"totalPages"`
}

func (m *ArticleModel) List(ctx context.Context, in ListArticlesInput) (*ListArticlesResult, error) {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	page := in.Page
	if page <= 0 {
		page = 1
	}
	values := url.Values{}
	values.Set("select", "*")
	values.Set("order", "publishedAt.desc.nullslast,createdAt.desc")
	values.Set("limit", fmt.Sprintf("%d", limit))
	values.Set("offset", fmt.Sprintf("%d", (page-1)*limit))
	if in.Status != "" && in.Status != "all" {
		values.Set("status", "eq."+in.Status)
	}
	if q := strings.TrimSpace(in.Query); q != "" {
		like := "*" + strings.ReplaceAll(q, ",", "") + "*"
		values.Set("or", fmt.Sprintf("(slug.ilike.%s,category.ilike.%s)", like, like))
	}
	var rows []articleRow
	resp, err := m.api.request(ctx, http.MethodGet, "Article", values, nil, "count=exact", &rows)
	if err != nil {
		return nil, err
	}
	items := make([]Article, 0, len(rows))
	for _, row := range rows {
		items = append(items, convertArticleRow(row))
	}
	if err := m.loadTranslations(ctx, items); err != nil {
		return nil, err
	}
	total := exactCount(resp, len(items))
	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}
	return &ListArticlesResult{Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages}, nil
}

func (m *ArticleModel) IsSlugTaken(ctx context.Context, slug, excludeID string) (bool, error) {
	values := url.Values{}
	values.Set("select", "id")
	values.Set("slug", "eq."+slug)
	values.Set("limit", "1")
	var rows []articleRow
	if _, err := m.api.request(ctx, http.MethodGet, "Article", values, nil, "", &rows); err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, nil
	}
	return rows[0].ID != excludeID, nil
}

func articleBody(id string, in ArticleInput) map[string]any {
	publishedAt := any(nil)
	if in.PublishedAt != nil {
		publishedAt = in.PublishedAt.UTC().Format(time.RFC3339)
	}
	body := map[string]any{"slug": in.Slug, "category": in.Category, "status": in.Status, "publishedAt": publishedAt, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	if id != "" {
		body["id"] = id
	}
	return body
}

func translationBodies(articleID string, translations []ArticleTranslation) ([]map[string]any, error) {
	bodies := make([]map[string]any, 0, len(translations))
	for _, t := range translations {
		sections, err := json.Marshal(t.Sections)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, map[string]any{
			"id":          newID(),
			"articleId":   articleID,
			"locale":      t.Locale,
			"title":       t.Title,
			"summary":     t.Summary,
			"lead":        t.Lead,
			"readingTime": t.ReadingTime,
			"content":     t.Content,
			"sections":    json.RawMessage(sections),
		})
	}
	return bodies, nil
}

func (m *ArticleModel) Create(ctx context.Context, in ArticleInput) (*Article, error) {
	id := newID()
	values := url.Values{}
	values.Set("select", "id")
	var rows []articleRow
	if _, err := m.api.request(ctx, http.MethodPost, "Article", values, articleBody(id, in), "return=representation", &rows); err != nil {
		return nil, err
	}
	bodies, err := translationBodies(id, in.Translations)
	if err != nil {
		return nil, err
	}
	if len(bodies) > 0 {
		if _, err := m.api.request(ctx, http.MethodPost, "ArticleTranslation", url.Values{}, bodies, "", nil); err != nil {
			return nil, err
		}
	}
	return m.FindByID(ctx, id)
}

func preserveMissingSections(
	existing []ArticleTranslation,
	incoming []ArticleTranslation,
) []ArticleTranslation {
	existingByLocale := make(map[string][]ArticleSection, len(existing))
	for _, translation := range existing {
		existingByLocale[translation.Locale] = translation.Sections
	}

	merged := make([]ArticleTranslation, 0, len(incoming))
	for _, translation := range incoming {
		if len(translation.Sections) == 0 {
			if sections, ok := existingByLocale[translation.Locale]; ok {
				translation.Sections = cloneArticleSections(sections)
			}
		}
		merged = append(merged, translation)
	}
	return merged
}

func cloneArticleSections(sections []ArticleSection) []ArticleSection {
	cloned := make([]ArticleSection, 0, len(sections))
	for _, section := range sections {
		section.Paragraphs = append([]string{}, section.Paragraphs...)
		cloned = append(cloned, section)
	}
	return cloned
}

func (m *ArticleModel) Update(ctx context.Context, id string, in ArticleInput) (*Article, error) {
	current, err := m.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrNotFound
	}
	in.Translations = preserveMissingSections(current.Translations, in.Translations)

	values := url.Values{}
	values.Set("id", "eq."+id)
	values.Set("select", "id")
	var rows []articleRow
	if _, err := m.api.request(ctx, http.MethodPatch, "Article", values, articleBody("", in), "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	del := url.Values{}
	del.Set("articleId", "eq."+id)
	if _, err := m.api.request(ctx, http.MethodDelete, "ArticleTranslation", del, nil, "", nil); err != nil {
		return nil, err
	}
	bodies, err := translationBodies(id, in.Translations)
	if err != nil {
		return nil, err
	}
	if len(bodies) > 0 {
		if _, err := m.api.request(ctx, http.MethodPost, "ArticleTranslation", url.Values{}, bodies, "", nil); err != nil {
			return nil, err
		}
	}
	return m.FindByID(ctx, id)
}

func (m *ArticleModel) Delete(ctx context.Context, id string) error {
	values := url.Values{}
	values.Set("id", "eq."+id)
	var rows []articleRow
	_, err := m.api.request(ctx, http.MethodDelete, "Article", values, nil, "return=representation", &rows)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return ErrNotFound
	}
	return nil
}
