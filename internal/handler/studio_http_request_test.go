package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

type studioHTTPDoerFunc func(req *http.Request) (*http.Response, error)

func (f studioHTTPDoerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type fakeStudioResolver struct {
	addresses map[string][]net.IPAddr
	err       error
}

func (r fakeStudioResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.addresses[host], nil
}

func TestValidateStudioHTTPRequestURL(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"file:///etc/passwd",
		"ftp://example.com/file",
		"http://user:pass@example.com",
		"http://localhost/admin",
		"http://127.0.0.1/admin",
		"http://[::1]/admin",
		"http://169.254.169.254/latest/meta-data",
		"https://example.com/data#fragment",
		"http://[fe80::1%25eth0]/",
		"http://example.com:70000/",
		"http://example.com:" + strings.Repeat("x", maxStudioHTTPURLBytes),
	} {
		if _, err := validateStudioHTTPRequestURL(raw); err == nil {
			t.Fatalf("expected %q to be rejected", raw)
		}
	}

	for _, raw := range []string{"https://example.com/data", "http://1.1.1.1/status"} {
		if _, err := validateStudioHTTPRequestURL(raw); err != nil {
			t.Fatalf("expected %q to be accepted: %v", raw, err)
		}
	}
}

func TestBlockedStudioOutboundIPRanges(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"0.0.0.0",
		"10.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"168.63.129.16",
		"169.254.169.254",
		"172.16.0.1",
		"192.0.0.1",
		"192.0.2.1",
		"192.168.1.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"224.0.0.1",
		"240.0.0.1",
		"::",
		"::1",
		"::ffff:127.0.0.1",
		"::ffff:10.0.0.1",
		"64:ff9b::127.0.0.1",
		"64:ff9b:1::1",
		"100::1",
		"2001:db8::1",
		"2002::1",
		"fc00::1",
		"fec0::1",
		"fe80::1",
	} {
		if !isBlockedStudioOutboundIP(net.ParseIP(raw)) {
			t.Fatalf("expected %s to be blocked", raw)
		}
	}

	for _, raw := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if isBlockedStudioOutboundIP(net.ParseIP(raw)) {
			t.Fatalf("expected %s to be allowed", raw)
		}
	}
}

func TestSafeStudioDialPinsValidatedPublicAddress(t *testing.T) {
	t.Parallel()

	resolver := fakeStudioResolver{addresses: map[string][]net.IPAddr{
		"public.example":  {{IP: net.ParseIP("1.1.1.1")}},
		"private.example": {{IP: net.ParseIP("10.0.0.5")}},
	}}
	var dialed string
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		dialed = address
		return nil, errors.New("dial stopped by test")
	}

	safeDial := newStudioSafeDialContext(resolver, dial)
	_, err := safeDial(context.Background(), "tcp", "public.example:443")
	if err == nil || dialed != "1.1.1.1:443" {
		t.Fatalf("expected validated IP to be dialed, address=%q err=%v", dialed, err)
	}

	dialed = ""
	_, err = safeDial(context.Background(), "tcp", "private.example:80")
	if err == nil || dialed != "" {
		t.Fatalf("private destination must fail before dial, address=%q err=%v", dialed, err)
	}
}

func TestSafeStudioHTTPClientUsesValidatedPinnedAddress(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "" || r.Header.Get("X-Test") != "safe" {
			t.Fatalf("unexpected upstream request host=%q headers=%v", r.Host, r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	_, port, err := net.SplitHostPort(upstream.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	resolver := fakeStudioResolver{addresses: map[string][]net.IPAddr{
		"public.example": {{IP: net.ParseIP("1.1.1.1")}},
	}}
	var pinnedAddress string
	realDialer := &net.Dialer{}
	client := newStudioSafeHTTPClientWithNetwork(resolver, func(ctx context.Context, network, address string) (net.Conn, error) {
		pinnedAddress = address
		return realDialer.DialContext(ctx, network, upstream.Listener.Addr().String())
	})
	req, err := http.NewRequest(http.MethodGet, "http://public.example:"+port+"/data", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "safe")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("safe client request failed: %v", err)
	}
	defer resp.Body.Close()
	body, err := readStudioHTTPResponseBody(resp.Body)
	if err != nil || string(body) != `{"ok":true}` {
		t.Fatalf("body=%q err=%v", body, err)
	}
	if pinnedAddress != net.JoinHostPort("1.1.1.1", port) {
		t.Fatalf("transport did not dial the validated address: %q", pinnedAddress)
	}
}

func TestSafeStudioDialFailsClosedForMixedDNSAnswers(t *testing.T) {
	t.Parallel()

	resolver := fakeStudioResolver{addresses: map[string][]net.IPAddr{
		"mixed.example": {{IP: net.ParseIP("1.1.1.1")}, {IP: net.ParseIP("127.0.0.1")}},
	}}
	dialCalled := false
	safeDial := newStudioSafeDialContext(resolver, func(context.Context, string, string) (net.Conn, error) {
		dialCalled = true
		return nil, errors.New("unexpected dial")
	})

	if _, err := safeDial(context.Background(), "tcp", "mixed.example:80"); err == nil || dialCalled {
		t.Fatalf("mixed public/private answers must fail closed, dialCalled=%v err=%v", dialCalled, err)
	}
}

func TestFilterStudioHTTPResponseHeadersRedactsAuthenticationMaterial(t *testing.T) {
	t.Parallel()

	headers := http.Header{
		"Content-Type":     {"application/json"},
		"Set-Cookie":       {"session=secret"},
		"WWW-Authenticate": {"Bearer realm=secret"},
		"Authorization":    {"Bearer secret"},
		"X-API-Key":        {"secret"},
		"X-Client-Secret":  {"secret"},
		"X-Ratelimit-abc":  {"1"},
	}
	filtered := filterStudioHTTPResponseHeaders(headers, []string{"secret", "abc"})
	if filtered.Get("Content-Type") != "application/json" || filtered.Get("Set-Cookie") != "" || filtered.Get("WWW-Authenticate") != "" || filtered.Get("Authorization") != "" || filtered.Get("X-API-Key") != "" || filtered.Get("X-Client-Secret") != "" || strings.Contains(strings.ToLower(fmt.Sprintf("%v", filtered)), "abc") {
		t.Fatalf("unexpected filtered headers: %#v", filtered)
	}
	body := map[string]any{"token": "prefix-secret-suffix", "secret": "safe-key-value", "items": []any{"secret", "safe"}}
	redacted := redactStudioCredentialValues(body, []string{"secret"}).(map[string]any)
	if strings.Contains(fmt.Sprintf("%v", redacted), "secret") || !strings.Contains(fmt.Sprintf("%v", redacted), "[REDACTED]") {
		t.Fatalf("credential value leaked after redaction: %#v", redacted)
	}
	status, _ := redactStudioCredentialValues("200 secret", []string{"secret"}).(string)
	if status != "200 [REDACTED]" {
		t.Fatalf("credential value leaked through HTTP reason phrase: %q", status)
	}
	short := redactStudioCredentialValues(map[string]any{"prefix-abc": "200 prefix-abc"}, []string{"abc"}).(map[string]any)
	if strings.Contains(fmt.Sprintf("%v", short), "abc") {
		t.Fatalf("short credential value leaked through key or reason phrase: %#v", short)
	}
}

func TestReadStudioHTTPResponseBodyIsBounded(t *testing.T) {
	t.Parallel()

	body, err := readStudioHTTPResponseBody(strings.NewReader("small response"))
	if err != nil || string(body) != "small response" {
		t.Fatalf("body=%q err=%v", body, err)
	}

	atLimit := bytes.NewReader(make([]byte, maxStudioHTTPResponseBytes))
	if body, err := readStudioHTTPResponseBody(atLimit); err != nil || len(body) != maxStudioHTTPResponseBytes {
		t.Fatalf("response at limit rejected: len=%d err=%v", len(body), err)
	}
	tooLarge := bytes.NewReader(make([]byte, maxStudioHTTPResponseBytes+1))
	if _, err := readStudioHTTPResponseBody(tooLarge); err == nil {
		t.Fatal("expected oversized response to be rejected")
	}
}

func TestValidateStudioHTTPHeaders(t *testing.T) {
	t.Parallel()

	if err := validateStudioHTTPHeaders(map[string]string{"Accept": "application/json", "X-Client-Version": "value"}); err != nil {
		t.Fatalf("safe headers rejected: %v", err)
	}
	for _, name := range []string{"Host", "Connection", "Content-Length", "Expect", "Keep-Alive", "Proxy-Authorization", "Transfer-Encoding", "X-Forwarded-For", "Authorization", "Cookie", "X-API-Key"} {
		if err := validateStudioHTTPHeaders(map[string]string{name: "unsafe"}); err == nil {
			t.Fatalf("expected %s to be rejected", name)
		}
	}
	for _, headers := range []map[string]string{
		{"X-Test": "one", "x-test": "two"},
		{"X-Test": "unsafe\x00value"},
		{"X-Test": "unsafe\r\nvalue"},
		{"X-One": strings.Repeat("a", maxStudioHTTPHeaderValueBytes), "X-Two": strings.Repeat("b", maxStudioHTTPHeaderValueBytes), "X-Three": strings.Repeat("c", maxStudioHTTPHeaderValueBytes), "X-Four": strings.Repeat("d", maxStudioHTTPHeaderValueBytes), "X-Five": "overflow"},
	} {
		if err := validateStudioHTTPHeaders(headers); err == nil {
			t.Fatalf("expected unsafe headers to be rejected: %#v", headers)
		}
	}

	if _, err := parseStudioHTTPHeaders(map[string]any{"X-Count": 7}); err == nil {
		t.Fatal("non-string header values must be rejected")
	}
	if headers, err := parseStudioHTTPHeaders(`{"Accept":"application/json"}`); err != nil || headers["Accept"] != "application/json" {
		t.Fatalf("valid JSON headers were not parsed: headers=%#v err=%v", headers, err)
	}
}

func TestStudioHTTPRequestBodyAndRedirectLimits(t *testing.T) {
	t.Parallel()

	if err := validateStudioHTTPRequestBody(strings.Repeat("x", maxStudioHTTPRequestBodyBytes)); err != nil {
		t.Fatalf("body at limit rejected: %v", err)
	}
	if err := validateStudioHTTPRequestBody(strings.Repeat("x", maxStudioHTTPRequestBodyBytes+1)); err == nil {
		t.Fatal("oversized request body must be rejected")
	}

	client := newStudioSafeHTTPClient()
	privateRedirect := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/private", nil)
	if err := client.CheckRedirect(privateRedirect, nil); err == nil {
		t.Fatal("redirect to a private destination must be rejected")
	}
	publicRedirect := httptest.NewRequest(http.MethodGet, "https://example.com/data", nil)
	allowedVia := make([]*http.Request, maxStudioHTTPRedirects)
	for index := range allowedVia {
		allowedVia[index] = httptest.NewRequest(http.MethodGet, "https://example.com/start", nil)
	}
	if err := client.CheckRedirect(publicRedirect, allowedVia); err != nil {
		t.Fatalf("redirect chain at configured limit was rejected: %v", err)
	}
	if err := client.CheckRedirect(publicRedirect, append(allowedVia, httptest.NewRequest(http.MethodGet, "https://example.com/start", nil))); err == nil {
		t.Fatal("redirect chain beyond the limit must be rejected")
	}
	original := httptest.NewRequest(http.MethodGet, "https://example.com/start", nil)
	crossHost := httptest.NewRequest(http.MethodGet, "https://redirected.example/data", nil)
	if err := client.CheckRedirect(crossHost, []*http.Request{original}); err == nil {
		t.Fatal("cross-host redirects must be rejected to prevent header leakage")
	}
	downgrade := httptest.NewRequest(http.MethodGet, "http://example.com/data", nil)
	if err := client.CheckRedirect(downgrade, []*http.Request{original}); err == nil {
		t.Fatal("https-to-http redirects must be rejected")
	}
}

func TestAdminExecuteStudioHTTPRequestSanitizesTransportErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"wf-2","name":"Public Request","description":"Demo","category":"Ops","status":"draft","runs":0,"success":0,"nodes":["Manual","HTTP Request"],"definition":{"version":1,"nodes":[{"id":"manual","type":"manual","kind":"trigger","label":"Manual","position":{"x":0,"y":0},"config":{"enabled":true}},{"id":"request","type":"http-request","kind":"action","label":"HTTP Request","position":{"x":200,"y":0},"config":{"method":"GET","url":"https://example.com/data?marker=must-not-leak","headers":{},"body":""}}],"edges":[{"id":"edge-manual-request","source":"manual","target":"request"}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	originalClient := studioSafeHTTPClient
	studioSafeHTTPClient = studioHTTPDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("transport detail must-not-leak")
	})
	defer func() { studioSafeHTTPClient = originalClient }()

	service := &svc.ServiceContext{
		Config:     config.Config{AdminApiToken: "test-token"},
		HasDatabse: true,
		Studio:     model.NewStudioModel(model.NewSupabaseREST(server.URL, "key")),
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/studio/workflows/wf-2/nodes/request/http-request", nil)
	request = pathvar.WithVars(request, map[string]string{"id": "wf-2", "nodeId": "request"})
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()

	AdminExecuteStudioHttpRequestHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "HTTP request failed.") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "must-not-leak") {
		t.Fatalf("transport details leaked in response: %s", recorder.Body.String())
	}
}

func TestAdminExecuteStudioHTTPRequestRejectsPrivateDestination(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/v1/StudioWorkflow" {
			t.Fatalf("unexpected persistence path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"wf-1","name":"Private Request","description":"Demo","category":"Ops","status":"draft","runs":0,"success":0,"nodes":["Manual","HTTP Request"],"definition":{"version":1,"nodes":[{"id":"manual","type":"manual","kind":"trigger","label":"Manual","position":{"x":0,"y":0},"config":{"enabled":true}},{"id":"request","type":"http-request","kind":"action","label":"HTTP Request","position":{"x":200,"y":0},"config":{"method":"GET","url":"http://169.254.169.254/latest/meta-data","headers":{},"body":""}}],"edges":[{"id":"edge-manual-request","source":"manual","target":"request"}]},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z"}]`))
	}))
	defer server.Close()

	service := &svc.ServiceContext{
		Config:     config.Config{AdminApiToken: "test-token"},
		HasDatabse: true,
		Studio:     model.NewStudioModel(model.NewSupabaseREST(server.URL, "key")),
	}
	request := httptest.NewRequest(http.MethodPost, "/api/admin/studio/workflows/wf-1/nodes/request/http-request", nil)
	request = pathvar.WithVars(request, map[string]string{"id": "wf-1", "nodeId": "request"})
	request.Header.Set("Authorization", "Bearer test-token")
	recorder := httptest.NewRecorder()

	AdminExecuteStudioHttpRequestHandler(service).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "destination is not allowed") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
