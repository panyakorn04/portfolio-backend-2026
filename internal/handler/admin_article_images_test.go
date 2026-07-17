package handler

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

func validPNG(t *testing.T) []byte {
	t.Helper()
	var contents bytes.Buffer
	canvas := image.NewRGBA(image.Rect(0, 0, 1, 1))
	canvas.Set(0, 0, color.RGBA{R: 255, A: 255})
	if err := png.Encode(&contents, canvas); err != nil {
		t.Fatal(err)
	}
	return contents.Bytes()
}

func articleImageUploadRequest(t *testing.T, name string, contents []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/admin/article-images", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", "Bearer test-token")
	return request
}

func TestAdminUploadArticleImageHandler(t *testing.T) {
	t.Parallel()

	storageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/storage/v1/object/article-images/articles/") {
			t.Fatalf("unexpected storage path %q", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "image/png" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"Key":"ok"}`))
	}))
	defer storageServer.Close()

	service := &svc.ServiceContext{
		Config:  config.Config{AdminApiToken: "test-token"},
		Storage: model.NewSupabaseStorage(storageServer.URL, "service-key"),
	}
	request := articleImageUploadRequest(t, "cover.png", validPNG(t))
	recorder := httptest.NewRecorder()

	AdminUploadArticleImageHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK || !strings.Contains(payload.Data.URL, "/storage/v1/object/public/article-images/articles/") {
		t.Fatalf("payload=%s", recorder.Body.String())
	}
}

func TestAdminUploadArticleImageHandlerRejectsNonImage(t *testing.T) {
	t.Parallel()
	service := &svc.ServiceContext{
		Config:  config.Config{AdminApiToken: "test-token"},
		Storage: model.NewSupabaseStorage("https://example.supabase.co", "service-key"),
	}
	request := articleImageUploadRequest(t, "notes.txt", []byte("not an image"))
	recorder := httptest.NewRecorder()

	AdminUploadArticleImageHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAdminUploadArticleImageHandlerRejectsTruncatedImage(t *testing.T) {
	t.Parallel()
	service := &svc.ServiceContext{
		Config:  config.Config{AdminApiToken: "test-token"},
		Storage: model.NewSupabaseStorage("https://example.supabase.co", "service-key"),
	}
	request := articleImageUploadRequest(t, "broken.png", []byte("\x89PNG\r\n\x1a\nrest"))
	recorder := httptest.NewRecorder()

	AdminUploadArticleImageHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestArticleImageUploadRateLimit(t *testing.T) {
	t.Parallel()
	service := &svc.ServiceContext{}
	access := &auth.AccessContext{Via: auth.ViaBearer}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/article-images", nil)
	request.RemoteAddr = "198.51.100.42:1234"
	request.Header.Set("Authorization", "Bearer image-upload-rate-test")

	for range articleImageUploadLimit {
		if recorder := httptest.NewRecorder(); !enforceArticleImageUploadRateLimit(recorder, request, service, access) {
			t.Fatalf("request within limit was blocked: status=%d", recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	if enforceArticleImageUploadRateLimit(recorder, request, service, access) {
		t.Fatal("request beyond limit was allowed")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
