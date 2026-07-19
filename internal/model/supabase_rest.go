package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SupabaseREST struct {
	baseURL string
	key     string
	http    *http.Client
}

func NewSupabaseREST(baseURL, key string) *SupabaseREST {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	key = strings.TrimSpace(key)
	if baseURL == "" || key == "" {
		return nil
	}
	return &SupabaseREST{baseURL: baseURL + "/rest/v1", key: key, http: &http.Client{Timeout: 15 * time.Second}}
}

func (c *SupabaseREST) request(ctx context.Context, method, table string, values url.Values, body any, prefer string, out any) (*http.Response, error) {
	u := c.baseURL + "/" + table
	if encoded := values.Encode(); encoded != "" {
		u += "?" + encoded
	}

	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", c.key)
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if prefer != "" {
		req.Header.Set("Prefer", prefer)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return resp, fmt.Errorf("supabase %s %s returned %s: %s", method, table, resp.Status, strings.TrimSpace(string(b)))
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp, err
		}
	} else {
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return resp, nil
}

func exactCount(resp *http.Response, fallback int) int {
	if resp == nil {
		return fallback
	}
	return exactCountFromHeader(resp.Header, fallback)
}

func exactCountFromHeader(header http.Header, fallback int) int {
	contentRange := header.Get("Content-Range")
	if contentRange == "" {
		return fallback
	}
	parts := strings.Split(contentRange, "/")
	if len(parts) != 2 || parts[1] == "*" {
		return fallback
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return fallback
	}
	return n
}

func timePtrFromString(raw *string) *time.Time {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	t := parseSupabaseTime(*raw)
	if t.IsZero() {
		return nil
	}
	return &t
}

func timeFromString(raw string) time.Time { return parseSupabaseTime(raw) }
