package model

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SupabaseStorage struct {
	baseURL string
	key     string
	http    *http.Client
}

func NewSupabaseStorage(baseURL, key string) *SupabaseStorage {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key = strings.TrimSpace(key)
	if baseURL == "" || key == "" {
		return nil
	}
	return &SupabaseStorage{
		baseURL: baseURL,
		key:     key,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func escapeStoragePath(value string) string {
	parts := strings.Split(strings.Trim(value, "/"), "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func (s *SupabaseStorage) Upload(
	ctx context.Context,
	bucket string,
	objectPath string,
	contentType string,
	body io.Reader,
) (string, error) {
	bucket = strings.TrimSpace(bucket)
	objectPath = strings.Trim(objectPath, "/")
	if bucket == "" || objectPath == "" {
		return "", fmt.Errorf("storage bucket and object path are required")
	}

	escapedBucket := url.PathEscape(bucket)
	escapedObjectPath := escapeStoragePath(objectPath)
	uploadURL := s.baseURL + "/storage/v1/object/" + escapedBucket + "/" + escapedObjectPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("apikey", s.key)
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "false")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("supabase storage upload returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	return s.baseURL + "/storage/v1/object/public/" + escapedBucket + "/" + escapedObjectPath, nil
}
