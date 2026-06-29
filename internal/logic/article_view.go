package logic

import (
	"portfolio-backend/internal/model"
)

type ArticleListItem struct {
	Slug        string `json:"slug"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Lead        string `json:"lead"`
	PublishedAt string `json:"publishedAt"`
	ReadingTime string `json:"readingTime"`
}

type ArticleDetail struct {
	Slug        string                 `json:"slug"`
	Category    string                 `json:"category"`
	Title       string                 `json:"title"`
	Summary     string                 `json:"summary"`
	Lead        string                 `json:"lead"`
	ReadingTime string                 `json:"readingTime"`
	PublishedAt string                 `json:"publishedAt"`
	Sections    []model.ArticleSection `json:"sections"`
}

// pickTranslation prefers the requested locale, then en, then the first available.
func pickTranslation(article *model.Article, locale string) *model.ArticleTranslation {
	var en, first *model.ArticleTranslation
	for i := range article.Translations {
		t := &article.Translations[i]
		if i == 0 {
			first = t
		}
		if t.Locale == locale {
			return t
		}
		if t.Locale == "en" {
			en = t
		}
	}
	if en != nil {
		return en
	}
	return first
}

func formatPublishedAt(article *model.Article) string {
	if article.PublishedAt == nil {
		return ""
	}
	return article.PublishedAt.UTC().Format("2006-01-02")
}

func ToListItem(article *model.Article, locale string) *ArticleListItem {
	t := pickTranslation(article, locale)
	if t == nil {
		return nil
	}
	return &ArticleListItem{
		Slug: article.Slug, Category: article.Category,
		Title: t.Title, Summary: t.Summary, Lead: t.Lead,
		PublishedAt: formatPublishedAt(article), ReadingTime: t.ReadingTime,
	}
}

func ToDetail(article *model.Article, locale string) *ArticleDetail {
	t := pickTranslation(article, locale)
	if t == nil {
		return nil
	}
	sections := t.Sections
	if sections == nil {
		sections = []model.ArticleSection{}
	}
	return &ArticleDetail{
		Slug: article.Slug, Category: article.Category,
		Title: t.Title, Summary: t.Summary, Lead: t.Lead,
		ReadingTime: t.ReadingTime, PublishedAt: formatPublishedAt(article),
		Sections: sections,
	}
}
