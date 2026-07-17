package handler

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"

	_ "golang.org/x/image/webp"
)

const (
	maxArticleImageBytes        = 5 << 20
	maxArticleImageRequestBytes = maxArticleImageBytes + (1 << 20)
	articleImageBucket          = "article-images"
	articleImageUploadLimit     = 20
	articleImageUploadWindow    = time.Minute
)

var articleImageTypes = map[string]string{
	"image/gif":  ".gif",
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var (
	errArticleImageTooLarge = errors.New("image exceeds 5 MB")
	errInvalidArticleImage  = errors.New("image is empty or has an unsupported type")
)

func readArticleImage(file multipart.File) ([]byte, string, error) {
	contents, err := io.ReadAll(io.LimitReader(file, maxArticleImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(contents) == 0 {
		return nil, "", errInvalidArticleImage
	}
	if len(contents) > maxArticleImageBytes {
		return nil, "", errArticleImageTooLarge
	}

	contentType := http.DetectContentType(contents)
	extension, allowed := articleImageTypes[contentType]
	if !allowed {
		return nil, "", errInvalidArticleImage
	}
	decoded, _, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil || decoded.Width < 1 || decoded.Height < 1 {
		return nil, "", errInvalidArticleImage
	}
	return contents, extension, nil
}

func enforceArticleImageUploadRateLimit(
	w http.ResponseWriter,
	r *http.Request,
	service *svc.ServiceContext,
	access *auth.AccessContext,
) bool {
	ip := clientIP(r, service != nil && service.Config.TrustProxy)
	keys := []string{"article-image:ip:" + ip}
	if access.Via == auth.ViaSession {
		raw := auth.GetCookieValue(r, auth.SessionCookieName)
		keys = append(keys, "article-image:session:"+auth.HashSessionToken(raw))
	} else {
		token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		keys = append(keys, "article-image:bearer:"+ratelimitKey(token))
	}
	for _, key := range keys {
		if blocked, retry := limited(service, key, articleImageUploadLimit, articleImageUploadWindow, time.Now()); blocked {
			writeRateLimited(w, retry)
			return false
		}
	}
	return true
}

func newArticleImagePath(extension string) (string, error) {
	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return "", err
	}
	now := time.Now().UTC()
	return fmt.Sprintf("articles/%04d/%02d/%s%s", now.Year(), now.Month(), hex.EncodeToString(identifier), extension), nil
}

// AdminUploadArticleImageHandler -> POST /api/admin/article-images
func AdminUploadArticleImageHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok {
			return
		}
		if !assertRole(w, access, []string{"admin", "editor"}) {
			return
		}
		if !enforceArticleImageUploadRateLimit(w, r, service, access) {
			return
		}
		if service.Storage == nil {
			response.Error(w, http.StatusServiceUnavailable, "Image storage is not configured.")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, maxArticleImageRequestBytes)
		if err := r.ParseMultipartForm(maxArticleImageBytes); err != nil {
			var maxBytesError *http.MaxBytesError
			if errors.As(err, &maxBytesError) {
				response.Error(w, http.StatusRequestEntityTooLarge, "Image must be 5 MB or smaller.")
				return
			}
			response.Error(w, http.StatusBadRequest, "Request must include an image file.")
			return
		}
		if r.MultipartForm != nil {
			defer func() {
				_ = r.MultipartForm.RemoveAll()
			}()
		}

		file, _, err := r.FormFile("image")
		if err != nil {
			response.Error(w, http.StatusBadRequest, "Request must include an image file.")
			return
		}
		defer file.Close()

		contents, extension, err := readArticleImage(file)
		if err != nil {
			if errors.Is(err, errArticleImageTooLarge) {
				response.Error(w, http.StatusRequestEntityTooLarge, "Image must be 5 MB or smaller.")
				return
			}
			response.Error(w, http.StatusBadRequest, "Use a PNG, JPEG, WebP, or GIF image.")
			return
		}
		objectPath, err := newArticleImagePath(extension)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to prepare image upload.")
			return
		}
		contentType := http.DetectContentType(contents)
		url, err := service.Storage.Upload(
			r.Context(),
			articleImageBucket,
			objectPath,
			contentType,
			bytes.NewReader(contents),
		)
		if err != nil {
			response.Error(w, http.StatusBadGateway, "Unable to upload image.")
			return
		}

		response.Ok(w, http.StatusCreated, map[string]string{"url": url})
	}
}
