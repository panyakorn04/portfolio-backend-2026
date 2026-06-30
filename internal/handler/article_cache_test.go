package handler

import "testing"

func TestArticleListCacheKey(t *testing.T) {
	limit := 3
	tests := []struct {
		name   string
		locale string
		limit  *int
		want   string
	}{
		{name: "all", locale: "en", limit: nil, want: "portfolio:articles:list:lang=en:limit=all"},
		{name: "limited", locale: "th", limit: &limit, want: "portfolio:articles:list:lang=th:limit=3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := articleListCacheKey(tt.locale, tt.limit); got != tt.want {
				t.Fatalf("articleListCacheKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArticleDetailCacheKeyEscapesSlug(t *testing.T) {
	got := articleDetailCacheKey("th", " my slug ")
	want := "portfolio:articles:detail:lang=th:slug=my+slug"
	if got != want {
		t.Fatalf("articleDetailCacheKey() = %q, want %q", got, want)
	}
}
