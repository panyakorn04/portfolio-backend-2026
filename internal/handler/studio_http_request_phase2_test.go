package handler

import (
	"net/http"
	"testing"
)

func TestParseStudioHTTPRequestConfigSupportsPhase2Contract(t *testing.T) {
	t.Parallel()

	config, err := parseStudioHTTPRequestConfig(map[string]any{
		"method": "POST",
		"url":    "https://example.com/events",
		"headers": map[string]any{
			"Content-Type": "application/json",
		},
		"body": "{\"ok\":true}",
		"queryParameters": []any{
			map[string]any{"name": "limit", "value": "10"},
			map[string]any{"name": "tag", "value": "go"},
		},
		"authMode":        "credential",
		"genericAuthType": "headerAuth",
		"credentialId":    "credential-1",
		"options": map[string]any{
			"timeoutMs":              float64(12000),
			"followRedirects":        false,
			"maxRedirects":           float64(2),
			"responseFormat":         "json",
			"includeResponseHeaders": false,
			"ignoreHttpStatusErrors": true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if config.Method != http.MethodPost || config.GenericAuthType != "headerAuth" || config.CredentialID != "credential-1" || len(config.QueryParameters) != 2 {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.Options.TimeoutMS != 12000 || config.Options.FollowRedirects || config.Options.MaxRedirects != 2 || config.Options.ResponseFormat != "json" || config.Options.IncludeResponseHeaders || !config.Options.IgnoreHTTPStatusErrors {
		t.Fatalf("unexpected options: %#v", config.Options)
	}
}

func TestParseStudioHTTPRequestConfigRejectsInvalidPhase2Values(t *testing.T) {
	t.Parallel()

	base := map[string]any{"method": "GET", "url": "https://example.com", "headers": map[string]any{}, "body": ""}
	cases := []map[string]any{
		mergeStudioConfig(base, map[string]any{"queryParameters": []any{map[string]any{"name": "", "value": "x"}}}),
		mergeStudioConfig(base, map[string]any{"authMode": "credential"}),
		mergeStudioConfig(base, map[string]any{"authMode": "credential", "credentialId": "credential-1"}),
		mergeStudioConfig(base, map[string]any{"authMode": "credential", "genericAuthType": "basicAuth", "credentialId": "credential-1"}),
		mergeStudioConfig(base, map[string]any{"authMode": "none", "genericAuthType": "headerAuth"}),
		mergeStudioConfig(base, map[string]any{"authMode": "inline-token"}),
		mergeStudioConfig(base, map[string]any{"options": map[string]any{"timeoutMs": float64(99)}}),
		mergeStudioConfig(base, map[string]any{"options": map[string]any{"maxRedirects": float64(20)}}),
		mergeStudioConfig(base, map[string]any{"options": map[string]any{"responseFormat": "xml"}}),
	}
	for index, raw := range cases {
		if _, err := parseStudioHTTPRequestConfig(raw); err == nil {
			t.Fatalf("case %d should fail: %#v", index, raw)
		}
	}
}

func TestGenericHeaderAuthRejectsOtherCredentialTypes(t *testing.T) {
	t.Parallel()
	if !studioCredentialMatchesGenericAuth("headerAuth", "header") {
		t.Fatal("header credential should match Header Auth")
	}
	for _, credentialType := range []string{"bearer", "basic", "query"} {
		if studioCredentialMatchesGenericAuth("headerAuth", credentialType) {
			t.Fatalf("credential type %q must not match Header Auth", credentialType)
		}
	}
	if studioCredentialMatchesGenericAuth("", "header") {
		t.Fatal("missing generic auth subtype must fail closed")
	}
}

func TestApplyStudioCredentialToRequest(t *testing.T) {
	t.Parallel()

	request, err := http.NewRequest(http.MethodGet, "https://example.com/data", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyStudioCredential(request, &studioResolvedCredential{Type: "bearer", Data: map[string]string{"token": "secret-token"}}); err != nil {
		t.Fatal(err)
	}
	if request.Header.Get("Authorization") != "Bearer secret-token" {
		t.Fatal("bearer credential was not applied")
	}

	queryRequest, _ := http.NewRequest(http.MethodGet, "https://example.com/data", nil)
	if err := applyStudioCredential(queryRequest, &studioResolvedCredential{Type: "query", Data: map[string]string{"name": "api_key", "value": "secret"}}); err != nil {
		t.Fatal(err)
	}
	if queryRequest.URL.Query().Get("api_key") != "secret" {
		t.Fatal("query credential was not applied")
	}

	conflictRequest, _ := http.NewRequest(http.MethodGet, "https://example.com/data?api_key=static", nil)
	if err := applyStudioCredential(conflictRequest, &studioResolvedCredential{Type: "query", Data: map[string]string{"name": "api_key", "value": "secret"}}); err == nil {
		t.Fatal("credential must not overwrite a static query parameter")
	}
}

func TestParseStudioCurlCommandSanitizesCredentials(t *testing.T) {
	t.Parallel()

	result, err := parseStudioCurlCommand(`curl -X POST 'https://example.com/items?limit=2&api_key=query-secret&client_secret=hidden&%20refresh_token=hidden' -H 'Content-Type: application/json' -H 'Authorization: Bearer secret' --data '{"name":"demo"}'`)
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != http.MethodPost || result.URL != "https://example.com/items" || result.Body != "" {
		t.Fatalf("unexpected import: %#v", result)
	}
	if len(result.QueryParameters) != 1 || result.QueryParameters[0].Name != "limit" || result.Headers["Authorization"] != "" || result.Headers["Content-Type"] != "application/json" {
		t.Fatalf("query or credential sanitization failed: %#v", result)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("credential removal must emit a warning")
	}
	headerResult, err := parseStudioCurlCommand(`curl 'https://example.com' -H 'X-Client-Secret: hidden' -H 'X-Webhook-Secret: hidden' -H 'X-Custom-Token: sk-live-example'`)
	if err != nil || len(headerResult.Headers) != 0 || len(headerResult.Warnings) < 3 {
		t.Fatalf("custom headers were not stripped fail-closed: result=%#v err=%v", headerResult, err)
	}
	bodyResult, err := parseStudioCurlCommand(`curl 'https://example.com' --data '{"payload":"sk-live-example"}'`)
	if err != nil || bodyResult.Body != "" || len(bodyResult.Warnings) == 0 {
		t.Fatalf("cURL body was not stripped fail-closed: result=%#v err=%v", bodyResult, err)
	}
}

func TestStudioHTTPRequestSecretPolicyAppliesToDrafts(t *testing.T) {
	t.Parallel()

	cases := []map[string]any{
		{"url": "https://example.com?client_secret=value"},
		{"url": "https://example.com?%20api_key=value"},
		{"queryParameters": []any{map[string]any{"name": "refresh_token", "value": "value"}}},
		{"headers": map[string]any{"X-Client-Secret": "value"}},
		{"headers": `{"refresh_token":"value"`},
		{"body": map[string]any{"password": "value"}},
		{"body": `{"nested":{"access_token":"value"}}`},
		{"body": "password=value"},
	}
	for index, config := range cases {
		if err := validateStudioHTTPRequestSecretFreeConfig(config); err == nil {
			t.Fatalf("case %d should reject persisted secret material: %#v", index, config)
		}
	}
}

func TestStudioHTTPClientUsesPhase2TimeoutAndRedirectOptions(t *testing.T) {
	t.Parallel()

	options := defaultStudioHTTPRequestOptions()
	options.TimeoutMS = 2500
	options.FollowRedirects = false
	client := newStudioSafeHTTPClientWithOptions(options)
	defer client.CloseIdleConnections()
	if client.Timeout.Milliseconds() != 2500 {
		t.Fatalf("unexpected timeout: %s", client.Timeout)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://example.com/next", nil)
	previous, _ := http.NewRequest(http.MethodGet, "https://example.com/start", nil)
	if err := client.CheckRedirect(request, []*http.Request{previous}); err != http.ErrUseLastResponse {
		t.Fatalf("redirects should be returned without following: %v", err)
	}
}

func mergeStudioConfig(base, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range extra {
		merged[key] = value
	}
	return merged
}
