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

type SupabaseArticleClient struct {
	baseURL string
	key     string
	http    *http.Client
}

func NewSupabaseArticleClient(baseURL, key string) *SupabaseArticleClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key = strings.TrimSpace(key)
	if baseURL == "" || key == "" {
		return nil
	}
	return &SupabaseArticleClient{
		baseURL: baseURL + "/rest/v1",
		key:     key,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *SupabaseArticleClient) request(ctx context.Context, table string, values url.Values, out any) error {
	u := c.baseURL + "/" + table
	if encoded := values.Encode(); encoded != "" {
		u += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("supabase %s returned %s", table, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type supabaseArticleRow struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Category    string  `json:"category"`
	Status      string  `json:"status"`
	PublishedAt *string `json:"publishedAt"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

type supabaseTranslationRow struct {
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

func parseSupabaseTime(raw string) time.Time {
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05.999",
		"2006-01-02T15:04:05",
		"2006-01-02",
	} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}

func convertSupabaseArticle(row supabaseArticleRow) Article {
	var publishedAt *time.Time
	if row.PublishedAt != nil && strings.TrimSpace(*row.PublishedAt) != "" {
		t := parseSupabaseTime(*row.PublishedAt)
		publishedAt = &t
	}
	return Article{
		ID:          row.ID,
		Slug:        row.Slug,
		Category:    row.Category,
		Status:      row.Status,
		PublishedAt: publishedAt,
		CreatedAt:   parseSupabaseTime(row.CreatedAt),
		UpdatedAt:   parseSupabaseTime(row.UpdatedAt),
	}
}

func convertSupabaseTranslation(row supabaseTranslationRow) (ArticleTranslation, error) {
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

func (c *SupabaseArticleClient) loadTranslations(ctx context.Context, articles []Article) error {
	if len(articles) == 0 {
		return nil
	}

	ids := make([]string, 0, len(articles))
	index := make(map[string]int, len(articles))
	for i := range articles {
		ids = append(ids, articles[i].ID)
		index[articles[i].ID] = i
	}

	values := url.Values{}
	values.Set("select", "*")
	values.Set("articleId", "in.("+strings.Join(ids, ",")+")")
	values.Set("order", "locale.asc")

	var rows []supabaseTranslationRow
	if err := c.request(ctx, "ArticleTranslation", values, &rows); err != nil {
		return err
	}

	for _, row := range rows {
		translation, err := convertSupabaseTranslation(row)
		if err != nil {
			return err
		}
		if i, ok := index[row.ArticleID]; ok {
			articles[i].Translations = append(articles[i].Translations, translation)
		}
	}

	for i := range articles {
		if articles[i].Translations == nil {
			articles[i].Translations = []ArticleTranslation{}
		}
	}
	return nil
}

func (c *SupabaseArticleClient) ListPublished(ctx context.Context, limit *int) ([]Article, error) {
	values := url.Values{}
	values.Set("select", "*")
	values.Set("status", "eq.published")
	values.Set("order", "publishedAt.desc.nullslast,createdAt.desc")
	if limit != nil {
		values.Set("limit", fmt.Sprintf("%d", *limit))
	}

	var rows []supabaseArticleRow
	if err := c.request(ctx, "Article", values, &rows); err != nil {
		return nil, err
	}

	articles := make([]Article, 0, len(rows))
	for _, row := range rows {
		articles = append(articles, convertSupabaseArticle(row))
	}
	if err := c.loadTranslations(ctx, articles); err != nil {
		return nil, err
	}
	return articles, nil
}

func (c *SupabaseArticleClient) FindPublishedBySlug(ctx context.Context, slug string) (*Article, error) {
	values := url.Values{}
	values.Set("select", "*")
	values.Set("slug", "eq."+slug)
	values.Set("status", "eq.published")
	values.Set("limit", "1")

	var rows []supabaseArticleRow
	if err := c.request(ctx, "Article", values, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	articles := []Article{convertSupabaseArticle(rows[0])}
	if err := c.loadTranslations(ctx, articles); err != nil {
		return nil, err
	}
	return &articles[0], nil
}
