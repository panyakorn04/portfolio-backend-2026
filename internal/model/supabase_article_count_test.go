package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSupabaseArticleClientCountsPublishedLocaleWithoutLoadingAllArticles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/ArticleTranslation" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("locale") != "eq.th" || query.Get("Article.status") != "eq.published" {
			t.Fatalf("unexpected filters: %s", r.URL.RawQuery)
		}
		if query.Get("limit") != "1" || query.Get("select") != "id,Article!inner(id)" {
			t.Fatalf("count query is not bounded: %s", r.URL.RawQuery)
		}
		if r.Header.Get("Prefer") != "count=exact" {
			t.Fatalf("Prefer = %q, want count=exact", r.Header.Get("Prefer"))
		}
		w.Header().Set("Content-Range", "0-0/42")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"translation-1"}]`))
	}))
	defer server.Close()

	client := NewSupabaseArticleClient(server.URL, "test-service-key")
	total, err := client.CountPublishedForLocale(context.Background(), "th")
	if err != nil {
		t.Fatalf("CountPublishedForLocale() error = %v", err)
	}
	if total != 42 {
		t.Fatalf("total = %d, want 42", total)
	}
}
