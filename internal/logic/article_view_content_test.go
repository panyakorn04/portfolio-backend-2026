package logic

import (
	"testing"

	"portfolio-backend/internal/model"
)

func TestToDetailIncludesMDXContent(t *testing.T) {
	t.Parallel()

	article := &model.Article{
		Slug:     "mdx-article",
		Category: "Engineering",
		Translations: []model.ArticleTranslation{
			{
				Locale:      "en",
				Title:       "MDX article",
				Summary:     "Summary",
				Lead:        "Lead",
				ReadingTime: "4 min",
				Content:     "# MDX article\n\nBody.",
			},
		},
	}

	detail := ToDetail(article, "en")
	if detail == nil {
		t.Fatal("ToDetail() = nil, want article detail")
	}
	if detail.Content != article.Translations[0].Content {
		t.Fatalf("Content = %q, want %q", detail.Content, article.Translations[0].Content)
	}
}
