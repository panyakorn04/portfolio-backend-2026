package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveStudioHTTPRequestExpressionsUsesFirstIncomingJSONItem(t *testing.T) {
	t.Parallel()

	config := studioHTTPRequestConfig{
		Method: "POST",
		URL:    "https://example.com/users/{{$json.user.id}}",
		Headers: map[string]string{
			"X-Tenant": "tenant-{{$json.tenant}}",
		},
		Body: `{"name":"{{$json.user.name}}","active":"{{$json.active}}","profile":"{{$json.user}}"}`,
		QueryParameters: []studioHTTPQueryParameter{
			{Name: "status", Value: "{{$json.status}}"},
		},
	}
	items := []map[string]any{
		{"json": map[string]any{
			"user":   map[string]any{"id": float64(42), "name": "Kwan Jai"},
			"tenant": "studio",
			"active": true,
			"status": "ready",
		}},
		{"json": map[string]any{"user": map[string]any{"id": float64(99)}}},
	}

	resolved, err := resolveStudioHTTPRequestExpressions(config, items)
	if err != nil {
		t.Fatalf("resolve expressions: %v", err)
	}
	if resolved.URL != "https://example.com/users/42" {
		t.Fatalf("unexpected URL: %q", resolved.URL)
	}
	if resolved.Headers["X-Tenant"] != "tenant-studio" || resolved.QueryParameters[0].Value != "ready" {
		t.Fatalf("unexpected scalar mapping: %#v %#v", resolved.Headers, resolved.QueryParameters)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(resolved.Body), &body); err != nil {
		t.Fatalf("resolved body is not JSON: %v", err)
	}
	if body["name"] != "Kwan Jai" || body["active"] != true {
		t.Fatalf("unexpected body mapping: %#v", body)
	}
	profile, ok := body["profile"].(map[string]any)
	if !ok || profile["id"] != float64(42) {
		t.Fatalf("exact body expression must preserve JSON value: %#v", body["profile"])
	}
	if config.Headers["X-Tenant"] != "tenant-{{$json.tenant}}" {
		t.Fatal("resolver mutated persisted config")
	}
}

func TestResolveStudioHTTPRequestExpressionsFailsClosed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config studioHTTPRequestConfig
		items  []map[string]any
		match  string
	}{
		{
			name:   "missing input",
			config: studioHTTPRequestConfig{URL: "https://example.com/{{$json.id}}"},
			match:  "incoming JSON item",
		},
		{
			name:   "missing path",
			config: studioHTTPRequestConfig{URL: "https://example.com/{{$json.missing}}"},
			items:  []map[string]any{{"json": map[string]any{"id": "one"}}},
			match:  "was not found",
		},
		{
			name:   "unsupported expression",
			config: studioHTTPRequestConfig{URL: "https://example.com/{{ $json.id + 1 }}"},
			items:  []map[string]any{{"json": map[string]any{"id": "one"}}},
			match:  "unsupported expression",
		},
		{
			name:   "object in header",
			config: studioHTTPRequestConfig{URL: "https://example.com", Headers: map[string]string{"X-Value": "{{$json.object}}"}},
			items:  []map[string]any{{"json": map[string]any{"object": map[string]any{"id": "one"}}}},
			match:  "scalar",
		},
		{
			name:   "URL traversal segment",
			config: studioHTTPRequestConfig{URL: "https://example.com/users/{{$json.id}}"},
			items:  []map[string]any{{"json": map[string]any{"id": ".."}}},
			match:  "traversal",
		},
		{
			name:   "expression in body key",
			config: studioHTTPRequestConfig{URL: "https://example.com", Body: `{"{{$json.key}}":"value"}`},
			items:  []map[string]any{{"json": map[string]any{"key": "name"}}},
			match:  "only in JSON body values",
		},
		{
			name:   "expanded URL exceeds limit",
			config: studioHTTPRequestConfig{URL: "https://example.com/{{$json.value}}"},
			items:  []map[string]any{{"json": map[string]any{"value": strings.Repeat("x", maxStudioHTTPURLBytes)}}},
			match:  "too long",
		},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := resolveStudioHTTPRequestExpressions(testCase.config, testCase.items)
			if err == nil || !strings.Contains(err.Error(), testCase.match) {
				t.Fatalf("expected error containing %q, got %v", testCase.match, err)
			}
		})
	}
}

func TestResolveStudioHTTPRequestURLExpressionEncodesStructureDelimiters(t *testing.T) {
	t.Parallel()
	config := studioHTTPRequestConfig{URL: "https://example.com/users/{{$json.id}}"}
	resolved, err := resolveStudioHTTPRequestExpressions(config, []map[string]any{{"json": map[string]any{
		"id": "42?api_key=attacker-value&action=delete",
	}}})
	if err != nil {
		t.Fatalf("resolve URL expression: %v", err)
	}
	if resolved.URL != "https://example.com/users/42%3Fapi_key%3Dattacker-value%26action%3Ddelete" {
		t.Fatalf("URL expression altered request structure: %q", resolved.URL)
	}
}

func TestParseStudioHTTPRequestConfigValidatesExpressionSyntax(t *testing.T) {
	t.Parallel()
	base := map[string]any{
		"method": "POST", "url": "https://example.com/items/{{$json.id}}",
		"headers": map[string]any{"X-Request-ID": "{{$json.request.id}}"},
		"body":    `{"name":"{{$json.name}}"}`, "queryParameters": []any{},
		"authMode": "none", "options": map[string]any{},
	}
	if _, err := parseStudioHTTPRequestConfig(base); err != nil {
		t.Fatalf("valid safe expressions were rejected: %v", err)
	}
	invalid := mergeStudioConfig(base, map[string]any{"url": "https://example.com/{{ $json.id + 1 }}"})
	if _, err := parseStudioHTTPRequestConfig(invalid); err == nil || !strings.Contains(err.Error(), "unsupported expression") {
		t.Fatalf("unsupported expression was not rejected at save-time parse: %v", err)
	}
	inlineQuery := mergeStudioConfig(base, map[string]any{"url": "https://example.com/items?id={{$json.id}}"})
	if _, err := parseStudioHTTPRequestConfig(inlineQuery); err == nil || !strings.Contains(err.Error(), "Query Parameters") {
		t.Fatalf("inline URL query expression was not rejected: %v", err)
	}
}

func TestParseStudioHTTPRequestConfigRejectsDynamicHost(t *testing.T) {
	t.Parallel()
	_, err := parseStudioHTTPRequestConfig(map[string]any{
		"method":          "GET",
		"url":             "https://{{$json.host}}/items",
		"headers":         map[string]any{},
		"body":            "",
		"queryParameters": []any{},
		"authMode":        "none",
		"options":         map[string]any{},
	})
	if err == nil {
		t.Fatal("expected dynamic host rejection")
	}
}
