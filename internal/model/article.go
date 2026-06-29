package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type ArticleModel struct {
	db *sql.DB
}

func NewArticleModel(db *sql.DB) *ArticleModel {
	return &ArticleModel{db: db}
}

const articleColumns = `"id", "slug", "category", "status", "publishedAt", "createdAt", "updatedAt"`

// loadTranslations fetches translations for the given article IDs in one query.
func (m *ArticleModel) loadTranslations(ctx context.Context, ids []string) (map[string][]ArticleTranslation, error) {
	result := make(map[string][]ArticleTranslation)
	if len(ids) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(
		`SELECT "articleId", "id", "locale", "title", "summary", "lead", "readingTime", "sections"
		 FROM "ArticleTranslation" WHERE "articleId" IN (%s)`,
		strings.Join(placeholders, ","))

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var articleID string
		var t ArticleTranslation
		var sectionsRaw []byte
		if err := rows.Scan(&articleID, &t.ID, &t.Locale, &t.Title, &t.Summary,
			&t.Lead, &t.ReadingTime, &sectionsRaw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sectionsRaw, &t.Sections); err != nil {
			return nil, err
		}
		result[articleID] = append(result[articleID], t)
	}
	return result, rows.Err()
}

func (m *ArticleModel) scanArticles(ctx context.Context, query string, args ...any) ([]Article, error) {
	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var articles []Article
	var ids []string
	for rows.Next() {
		var a Article
		if err := rows.Scan(&a.ID, &a.Slug, &a.Category, &a.Status,
			&a.PublishedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		articles = append(articles, a)
		ids = append(ids, a.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	translations, err := m.loadTranslations(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range articles {
		articles[i].Translations = translations[articles[i].ID]
		if articles[i].Translations == nil {
			articles[i].Translations = []ArticleTranslation{}
		}
	}
	return articles, nil
}

func (m *ArticleModel) ListPublished(ctx context.Context, limit *int) ([]Article, error) {
	query := `SELECT ` + articleColumns + ` FROM "Article" WHERE "status" = 'published'
		ORDER BY "publishedAt" DESC NULLS LAST, "createdAt" DESC`
	args := []any{}
	if limit != nil {
		query += " LIMIT $1"
		args = append(args, *limit)
	}
	return m.scanArticles(ctx, query, args...)
}

func (m *ArticleModel) FindPublishedBySlug(ctx context.Context, slug string) (*Article, error) {
	query := `SELECT ` + articleColumns + ` FROM "Article"
		WHERE "slug" = $1 AND "status" = 'published' LIMIT 1`
	articles, err := m.scanArticles(ctx, query, slug)
	if err != nil {
		return nil, err
	}
	if len(articles) == 0 {
		return nil, nil
	}
	return &articles[0], nil
}

func (m *ArticleModel) FindByID(ctx context.Context, id string) (*Article, error) {
	query := `SELECT ` + articleColumns + ` FROM "Article" WHERE "id" = $1 LIMIT 1`
	articles, err := m.scanArticles(ctx, query, id)
	if err != nil {
		return nil, err
	}
	if len(articles) == 0 {
		return nil, nil
	}
	return &articles[0], nil
}

type ListArticlesInput struct {
	Limit  int
	Page   int
	Status string // "all" or specific
	Query  string
}

type ListArticlesResult struct {
	Items      []Article `json:"items"`
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	Limit      int       `json:"limit"`
	TotalPages int       `json:"totalPages"`
}

func buildArticleWhere(in ListArticlesInput) (string, []any) {
	var clauses []string
	var args []any
	idx := 1

	if in.Status != "" && in.Status != "all" {
		clauses = append(clauses, fmt.Sprintf(`a."status" = $%d`, idx))
		args = append(args, in.Status)
		idx++
	}

	if q := strings.TrimSpace(in.Query); q != "" {
		like := "%" + q + "%"
		clauses = append(clauses, fmt.Sprintf(
			`(a."slug" ILIKE $%d OR a."category" ILIKE $%d OR EXISTS (
				SELECT 1 FROM "ArticleTranslation" t
				WHERE t."articleId" = a."id" AND t."title" ILIKE $%d))`,
			idx, idx, idx))
		args = append(args, like)
	}

	if len(clauses) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
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

	where, args := buildArticleWhere(in)

	var total int
	countQuery := `SELECT count(*) FROM "Article" a ` + where
	if err := m.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}

	listQuery := fmt.Sprintf(
		`SELECT a."id", a."slug", a."category", a."status", a."publishedAt", a."createdAt", a."updatedAt"
		 FROM "Article" a %s
		 ORDER BY a."publishedAt" DESC NULLS LAST, a."createdAt" DESC
		 LIMIT $%d OFFSET $%d`,
		where, len(args)+1, len(args)+2)
	listArgs := append(append([]any{}, args...), limit, (page-1)*limit)

	items, err := m.scanArticles(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []Article{}
	}

	totalPages := (total + limit - 1) / limit
	if totalPages < 1 {
		totalPages = 1
	}

	return &ListArticlesResult{
		Items: items, Total: total, Page: page, Limit: limit, TotalPages: totalPages,
	}, nil
}

// IsSlugTaken reports whether the slug is used by an article other than excludeID.
func (m *ArticleModel) IsSlugTaken(ctx context.Context, slug, excludeID string) (bool, error) {
	var id string
	err := m.db.QueryRowContext(ctx, `SELECT "id" FROM "Article" WHERE "slug" = $1`, slug).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return id != excludeID, nil
}

func insertTranslations(ctx context.Context, tx *sql.Tx, articleID string, translations []ArticleTranslation) error {
	const query = `INSERT INTO "ArticleTranslation"
		("id", "articleId", "locale", "title", "summary", "lead", "readingTime", "sections")
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	for _, t := range translations {
		sections, err := json.Marshal(t.Sections)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, query,
			newID(), articleID, t.Locale, t.Title, t.Summary, t.Lead, t.ReadingTime, sections); err != nil {
			return err
		}
	}
	return nil
}

func (m *ArticleModel) Create(ctx context.Context, in ArticleInput) (*Article, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	id := newID()
	const insert = `INSERT INTO "Article" ("id", "slug", "category", "status", "publishedAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, now())`
	if _, err := tx.ExecContext(ctx, insert,
		id, in.Slug, in.Category, in.Status, in.PublishedAt); err != nil {
		return nil, err
	}

	if err := insertTranslations(ctx, tx, id, in.Translations); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m.FindByID(ctx, id)
}

// Update replaces the article and all its translations. Returns ErrNotFound when missing.
func (m *ArticleModel) Update(ctx context.Context, id string, in ArticleInput) (*Article, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	const update = `UPDATE "Article"
		SET "slug" = $1, "category" = $2, "status" = $3, "publishedAt" = $4, "updatedAt" = now()
		WHERE "id" = $5`
	res, err := tx.ExecContext(ctx, update,
		in.Slug, in.Category, in.Status, in.PublishedAt, id)
	if err != nil {
		return nil, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrNotFound
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM "ArticleTranslation" WHERE "articleId" = $1`, id); err != nil {
		return nil, err
	}
	if err := insertTranslations(ctx, tx, id, in.Translations); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m.FindByID(ctx, id)
}

// Delete removes an article. Returns ErrNotFound when missing.
func (m *ArticleModel) Delete(ctx context.Context, id string) error {
	res, err := m.db.ExecContext(ctx, `DELETE FROM "Article" WHERE "id" = $1`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
