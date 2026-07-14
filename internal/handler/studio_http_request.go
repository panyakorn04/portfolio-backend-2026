package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxStudioHTTPURLBytes         = 4 << 10
	maxStudioHTTPRequestBodyBytes = 1 << 20
	maxStudioHTTPResponseBytes    = 5 << 20
	maxStudioHTTPResponseHeaders  = 64 << 10
	maxStudioHTTPHeaderCount      = 50
	maxStudioHTTPHeaderNameBytes  = 128
	maxStudioHTTPHeaderValueBytes = 8 << 10
	maxStudioHTTPHeadersBytes     = 32 << 10
	maxStudioHTTPRedirects        = 5
)

var (
	errStudioHTTPDestinationBlocked                = errors.New("HTTP request destination is not allowed")
	errStudioHTTPResolutionFailed                  = errors.New("HTTP request destination could not be resolved")
	errStudioHTTPResponseTooLarge                  = errors.New("HTTP response exceeded the configured size limit")
	studioSafeHTTPClient            studioHTTPDoer = newStudioSafeHTTPClient()
	blockedStudioOutboundPrefixes                  = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("127.0.0.0/8"),
		netip.MustParsePrefix("168.63.129.16/32"),
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("172.16.0.0/12"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.88.99.0/24"),
		netip.MustParsePrefix("192.168.0.0/16"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("224.0.0.0/4"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("::/96"),
		netip.MustParsePrefix("64:ff9b::/96"),
		netip.MustParsePrefix("64:ff9b:1::/48"),
		netip.MustParsePrefix("100::/64"),
		netip.MustParsePrefix("2001::/23"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("2002::/16"),
		netip.MustParsePrefix("3fff::/20"),
		netip.MustParsePrefix("5f00::/16"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("ff00::/8"),
	}
)

type studioHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

type studioHostResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type studioDialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

func validateStudioHTTPRequestURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxStudioHTTPURLBytes {
		return nil, errors.New("HTTP request URL is invalid")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" {
		return nil, errors.New("HTTP request URL is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("HTTP request URL must use http or https")
	}
	if parsed.User != nil {
		return nil, errors.New("HTTP request URL must not contain embedded credentials")
	}
	if parsed.Fragment != "" {
		return nil, errors.New("HTTP request URL must not contain a fragment")
	}
	hostname := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if hostname == "" || strings.Contains(hostname, "%") {
		return nil, errors.New("HTTP request URL must include a valid hostname")
	}
	if port := parsed.Port(); port != "" {
		portNumber, portErr := strconv.Atoi(port)
		if portErr != nil || portNumber < 1 || portNumber > 65535 {
			return nil, errors.New("HTTP request URL has an invalid port")
		}
	}
	if hostname == "localhost" || strings.HasSuffix(hostname, ".localhost") {
		return nil, errStudioHTTPDestinationBlocked
	}
	if ip := net.ParseIP(hostname); ip != nil && isBlockedStudioOutboundIP(ip) {
		return nil, errStudioHTTPDestinationBlocked
	}
	return parsed, nil
}

func isBlockedStudioOutboundIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	for _, prefix := range blockedStudioOutboundPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return !address.IsGlobalUnicast()
}

func resolveStudioPublicIPs(ctx context.Context, resolver studioHostResolver, hostname string) ([]net.IP, error) {
	if ip := net.ParseIP(hostname); ip != nil {
		if isBlockedStudioOutboundIP(ip) {
			return nil, errStudioHTTPDestinationBlocked
		}
		return []net.IP{ip}, nil
	}
	addresses, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return nil, errStudioHTTPResolutionFailed
	}
	if len(addresses) == 0 {
		return nil, errStudioHTTPResolutionFailed
	}
	resolved := make([]net.IP, 0, len(addresses))
	for _, address := range addresses {
		if isBlockedStudioOutboundIP(address.IP) {
			return nil, errStudioHTTPDestinationBlocked
		}
		resolved = append(resolved, address.IP)
	}
	return resolved, nil
}

func newStudioSafeDialContext(resolver studioHostResolver, dial studioDialContextFunc) studioDialContextFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		hostname, port, err := net.SplitHostPort(address)
		if err != nil || hostname == "" || port == "" {
			return nil, errors.New("HTTP request destination is invalid")
		}
		addresses, err := resolveStudioPublicIPs(ctx, resolver, hostname)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range addresses {
			connection, dialErr := dial(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return connection, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = errors.New("HTTP request destination could not be reached")
		}
		return nil, lastErr
	}
}

func newStudioSafeHTTPClient() *http.Client {
	return newStudioSafeHTTPClientWithOptions(defaultStudioHTTPRequestOptions())
}

func newStudioSafeHTTPClientWithOptions(options studioHTTPRequestOptions) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return newStudioSafeHTTPClientWithNetworkOptions(net.DefaultResolver, dialer.DialContext, options)
}

func newStudioSafeHTTPClientWithNetwork(resolver studioHostResolver, dial studioDialContextFunc) *http.Client {
	return newStudioSafeHTTPClientWithNetworkOptions(resolver, dial, defaultStudioHTTPRequestOptions())
}

func newStudioSafeHTTPClientWithNetworkOptions(resolver studioHostResolver, dial studioDialContextFunc, options studioHTTPRequestOptions) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = newStudioSafeDialContext(resolver, dial)
	transport.MaxResponseHeaderBytes = maxStudioHTTPResponseHeaders
	transport.MaxConnsPerHost = 10
	transport.MaxIdleConnsPerHost = 5
	transport.IdleConnTimeout = 30 * time.Second
	transport.ResponseHeaderTimeout = 15 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	return &http.Client{
		Transport: transport,
		Timeout:   time.Duration(options.TimeoutMS) * time.Millisecond,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !options.FollowRedirects || options.MaxRedirects == 0 {
				return http.ErrUseLastResponse
			}
			if len(via) > options.MaxRedirects {
				return errors.New("HTTP request exceeded the redirect limit")
			}
			if len(via) > 0 {
				origin := via[0].URL
				if !strings.EqualFold(req.URL.Host, origin.Host) {
					return fmt.Errorf("%w: cross-host redirect", errStudioHTTPDestinationBlocked)
				}
				if origin.Scheme == "https" && req.URL.Scheme != "https" {
					return fmt.Errorf("%w: HTTPS downgrade redirect", errStudioHTTPDestinationBlocked)
				}
			}
			_, err := validateStudioHTTPRequestURL(req.URL.String())
			return err
		},
	}
}

func parseStudioHTTPHeaders(value any) (map[string]string, error) {
	switch headers := value.(type) {
	case nil:
		return nil, nil
	case map[string]string:
		return headers, nil
	case map[string]any:
		parsed := make(map[string]string, len(headers))
		for name, rawValue := range headers {
			value, ok := rawValue.(string)
			if !ok {
				return nil, fmt.Errorf("HTTP request header %q must have a string value", name)
			}
			parsed[name] = value
		}
		return parsed, nil
	case string:
		if strings.TrimSpace(headers) == "" {
			return nil, nil
		}
		var parsed map[string]string
		if err := json.Unmarshal([]byte(headers), &parsed); err != nil {
			return nil, errors.New("HTTP request headers must be a JSON object with string values")
		}
		return parsed, nil
	default:
		return nil, errors.New("HTTP request headers must be an object")
	}
}

func validateStudioHTTPHeaders(headers map[string]string) error {
	return validateStudioHTTPHeadersWithSecrets(headers, false)
}

func validateStudioCredentialHeaders(headers map[string]string) error {
	return validateStudioHTTPHeadersWithSecrets(headers, true)
}

func validateStudioHTTPHeadersWithSecrets(headers map[string]string, allowSensitive bool) error {
	if len(headers) > maxStudioHTTPHeaderCount {
		return fmt.Errorf("HTTP request headers must not exceed %d entries", maxStudioHTTPHeaderCount)
	}
	blocked := map[string]bool{
		"connection":          true,
		"content-length":      true,
		"expect":              true,
		"forwarded":           true,
		"host":                true,
		"keep-alive":          true,
		"proxy-authorization": true,
		"proxy-connection":    true,
		"te":                  true,
		"trailer":             true,
		"transfer-encoding":   true,
		"upgrade":             true,
	}
	sensitive := map[string]bool{
		"api-key":        true,
		"api-token":      true,
		"authorization":  true,
		"cookie":         true,
		"set-cookie":     true,
		"x-access-token": true,
		"x-api-key":      true,
		"x-api-token":    true,
		"x-auth":         true,
		"x-auth-token":   true,
		"x-secret":       true,
		"x-token":        true,
	}
	seen := make(map[string]bool, len(headers))
	totalBytes := 0
	for name, value := range headers {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		lowerName := strings.ToLower(canonical)
		if canonical == "" || len(name) > maxStudioHTTPHeaderNameBytes || blocked[lowerName] || (!allowSensitive && sensitive[lowerName]) || strings.HasPrefix(lowerName, "x-forwarded-") {
			return fmt.Errorf("HTTP request header %q is not allowed", name)
		}
		if seen[lowerName] {
			return fmt.Errorf("HTTP request header %q is duplicated", name)
		}
		seen[lowerName] = true
		if len(value) > maxStudioHTTPHeaderValueBytes || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("HTTP request header %q has an invalid value", name)
		}
		totalBytes += len(name) + len(value)
		if totalBytes > maxStudioHTTPHeadersBytes {
			return fmt.Errorf("HTTP request headers must not exceed %d bytes", maxStudioHTTPHeadersBytes)
		}
	}
	return nil
}

func validateStudioHTTPRequestBody(body string) error {
	if len(body) > maxStudioHTTPRequestBodyBytes {
		return fmt.Errorf("HTTP request body must not exceed %d bytes", maxStudioHTTPRequestBodyBytes)
	}
	return nil
}

func studioCredentialSecretValues(data map[string]string) []string {
	values := make([]string, 0, len(data))
	for name, value := range data {
		if name == "name" || value == "" {
			continue
		}
		values = append(values, value)
	}
	return values
}

func redactStudioCredentialValues(value any, secrets []string) any {
	switch typed := value.(type) {
	case string:
		for _, secret := range secrets {
			if typed == secret {
				return "[REDACTED]"
			}
			if len(secret) >= 4 {
				typed = strings.ReplaceAll(typed, secret, "[REDACTED]")
			}
		}
		return typed
	case map[string]any:
		for key, child := range typed {
			typed[key] = redactStudioCredentialValues(child, secrets)
		}
	case []any:
		for index, child := range typed {
			typed[index] = redactStudioCredentialValues(child, secrets)
		}
	}
	return value
}

func filterStudioHTTPResponseHeaders(headers http.Header, secrets []string) http.Header {
	filtered := make(http.Header, len(headers))
	blocked := map[string]bool{
		"authentication-info":       true,
		"proxy-authenticate":        true,
		"proxy-authentication-info": true,
		"set-cookie":                true,
		"www-authenticate":          true,
	}
	for name, values := range headers {
		if blocked[strings.ToLower(name)] {
			continue
		}
		redacted := make([]string, len(values))
		for index, value := range values {
			redacted[index], _ = redactStudioCredentialValues(value, secrets).(string)
		}
		filtered[name] = redacted
	}
	return filtered
}

func readStudioHTTPResponseBody(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, maxStudioHTTPResponseBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxStudioHTTPResponseBytes {
		return nil, errStudioHTTPResponseTooLarge
	}
	return body, nil
}
