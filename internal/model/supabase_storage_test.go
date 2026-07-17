package model

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSupabaseStorageUpload(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/storage/v1/object/article-images/articles/cover.png" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer service-key" || r.Header.Get("apikey") != "service-key" {
			t.Fatal("storage request is missing service-role headers")
		}
		if r.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "png-bytes" {
			t.Fatalf("body = %q", body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Key":"article-images/articles/cover.png"}`))
	}))
	defer server.Close()

	storage := NewSupabaseStorage(server.URL, "service-key")
	url, err := storage.Upload(context.Background(), "article-images", "articles/cover.png", "image/png", strings.NewReader("png-bytes"))
	if err != nil {
		t.Fatal(err)
	}
	want := server.URL + "/storage/v1/object/public/article-images/articles/cover.png"
	if url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}
}
