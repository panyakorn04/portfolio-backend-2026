package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const (
	maxStudioHTTPQueryParameters = 50
	maxStudioHTTPQueryNameBytes  = 256
	maxStudioHTTPQueryValueBytes = 8 << 10
	maxStudioCurlCommandBytes    = 16 << 10
)

type studioHTTPQueryParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type studioHTTPRequestOptions struct {
	TimeoutMS              int    `json:"timeoutMs"`
	FollowRedirects        bool   `json:"followRedirects"`
	MaxRedirects           int    `json:"maxRedirects"`
	ResponseFormat         string `json:"responseFormat"`
	IncludeResponseHeaders bool   `json:"includeResponseHeaders"`
	IgnoreHTTPStatusErrors bool   `json:"ignoreHttpStatusErrors"`
}

type studioHTTPRequestConfig struct {
	Method          string
	URL             string
	Headers         map[string]string
	Body            string
	QueryParameters []studioHTTPQueryParameter
	AuthMode        string
	GenericAuthType string
	CredentialID    string
	Options         studioHTTPRequestOptions
}

type studioResolvedCredential struct {
	Type string
	Data map[string]string
}

type studioCurlImportResult struct {
	Method          string                     `json:"method"`
	URL             string                     `json:"url"`
	Headers         map[string]string          `json:"headers"`
	Body            string                     `json:"body"`
	QueryParameters []studioHTTPQueryParameter `json:"queryParameters"`
	Warnings        []string                   `json:"warnings"`
}

func defaultStudioHTTPRequestOptions() studioHTTPRequestOptions {
	return studioHTTPRequestOptions{
		TimeoutMS:              30000,
		FollowRedirects:        true,
		MaxRedirects:           maxStudioHTTPRedirects,
		ResponseFormat:         "auto",
		IncludeResponseHeaders: true,
		IgnoreHTTPStatusErrors: true,
	}
}

func normalizeStudioSecretFieldName(name string) string {
	var normalized strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			normalized.WriteRune(char)
		}
	}
	return normalized.String()
}

func isStudioSecretFieldName(name string) bool {
	normalized := normalizeStudioSecretFieldName(name)
	switch normalized {
	case "authorization", "cookie", "apikey", "apitoken", "token", "accesstoken", "refreshtoken", "idtoken", "authtoken", "clientsecret", "password", "passwd", "secret", "signature":
		return true
	}
	return strings.HasSuffix(normalized, "apikey") || strings.HasSuffix(normalized, "apitoken") || strings.HasSuffix(normalized, "accesstoken") || strings.HasSuffix(normalized, "refreshtoken") || strings.HasSuffix(normalized, "clientsecret")
}

func containsStudioSecretJSONKey(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isStudioSecretFieldName(key) || containsStudioSecretJSONKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsStudioSecretJSONKey(child) {
				return true
			}
		}
	}
	return false
}

func validateStudioHTTPRequestSecretFreeConfig(raw map[string]any) error {
	if containsStudioSecretJSONKey(raw) {
		return errors.New("HTTP request secrets must use a saved credential reference")
	}
	if rawURL, ok := raw["url"].(string); ok && strings.TrimSpace(rawURL) != "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			for name := range parsed.Query() {
				if isStudioSecretFieldName(name) {
					return errors.New("HTTP request secrets must use a saved credential reference")
				}
			}
		}
	}
	if rawQuery, exists := raw["queryParameters"]; exists {
		if items, ok := rawQuery.([]any); ok {
			for _, item := range items {
				if record, ok := item.(map[string]any); ok {
					if name, ok := record["name"].(string); ok && isStudioSecretFieldName(name) {
						return errors.New("HTTP request secrets must use a saved credential reference")
					}
				}
			}
		}
	}
	if rawHeaders, exists := raw["headers"]; exists {
		headers, err := parseStudioHTTPHeaders(rawHeaders)
		if err == nil {
			for name := range headers {
				if isStudioSecretFieldName(name) {
					return errors.New("HTTP request secrets must use a saved credential reference")
				}
			}
			if err := validateStudioHTTPHeaders(headers); err != nil {
				return err
			}
		} else if text, ok := rawHeaders.(string); ok {
			compact := normalizeStudioSecretFieldName(text)
			for _, marker := range []string{"authorization", "clientsecret", "accesstoken", "refreshtoken", "idtoken", "authtoken", "apikey", "apitoken", "password", "passwd", "cookie", "signature"} {
				if strings.Contains(compact, marker) {
					return errors.New("HTTP request secrets must use a saved credential reference")
				}
			}
		}
	}
	if body, ok := raw["body"].(string); ok && strings.TrimSpace(body) != "" {
		var decoded any
		if json.Unmarshal([]byte(body), &decoded) == nil && containsStudioSecretJSONKey(decoded) {
			return errors.New("HTTP request secrets must use a saved credential reference")
		}
		if values, err := url.ParseQuery(body); err == nil {
			for name := range values {
				if isStudioSecretFieldName(name) {
					return errors.New("HTTP request secrets must use a saved credential reference")
				}
			}
		}
	}
	return nil
}

func parseStudioHTTPRequestConfig(raw map[string]any) (studioHTTPRequestConfig, error) {
	if err := validateStudioHTTPRequestSecretFreeConfig(raw); err != nil {
		return studioHTTPRequestConfig{}, err
	}
	method, methodOK := raw["method"].(string)
	rawURL, urlOK := raw["url"].(string)
	validMethods := map[string]bool{
		http.MethodGet: true, http.MethodPost: true, http.MethodPut: true,
		http.MethodPatch: true, http.MethodDelete: true,
	}
	if !methodOK || !validMethods[method] || !urlOK {
		return studioHTTPRequestConfig{}, errors.New("HTTP request method or URL is invalid")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil || len(rawURL) == 0 || len(rawURL) > maxStudioHTTPURLBytes || parsedURL.Opaque != "" || parsedURL.Fragment != "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" || parsedURL.User != nil {
		return studioHTTPRequestConfig{}, errors.New("HTTP request URL is invalid")
	}
	if studioStringHasExpression(parsedURL.Scheme) || studioStringHasExpression(parsedURL.Host) {
		return studioHTTPRequestConfig{}, errors.New("HTTP request URL host cannot use expressions")
	}

	headers, err := parseStudioHTTPHeaders(raw["headers"])
	if err != nil {
		return studioHTTPRequestConfig{}, err
	}
	if err := validateStudioHTTPHeaders(headers); err != nil {
		return studioHTTPRequestConfig{}, err
	}

	body := ""
	if rawBody, exists := raw["body"]; exists {
		var ok bool
		body, ok = rawBody.(string)
		if !ok {
			return studioHTTPRequestConfig{}, errors.New("HTTP request body must be a string")
		}
	}
	if err := validateStudioHTTPRequestBody(body); err != nil {
		return studioHTTPRequestConfig{}, err
	}
	if (method == http.MethodGet || method == http.MethodDelete) && body != "" {
		return studioHTTPRequestConfig{}, errors.New("GET and DELETE requests must not include a body")
	}

	query, err := parseStudioHTTPQueryParameters(raw["queryParameters"])
	if err != nil {
		return studioHTTPRequestConfig{}, err
	}
	authMode := "none"
	if value, exists := raw["authMode"]; exists {
		var ok bool
		authMode, ok = value.(string)
		if !ok || (authMode != "none" && authMode != "credential") {
			return studioHTTPRequestConfig{}, errors.New("HTTP request authentication mode is invalid")
		}
	}
	credentialID, _ := raw["credentialId"].(string)
	credentialID = strings.TrimSpace(credentialID)
	genericAuthType := ""
	if value, exists := raw["genericAuthType"]; exists {
		var ok bool
		genericAuthType, ok = value.(string)
		if !ok || genericAuthType != "headerAuth" {
			return studioHTTPRequestConfig{}, errors.New("HTTP request generic authentication type is invalid")
		}
	}
	if authMode == "credential" && (credentialID == "" || len(credentialID) > 128) {
		return studioHTTPRequestConfig{}, errors.New("HTTP request credential is required")
	}
	if authMode == "credential" && genericAuthType != "headerAuth" {
		return studioHTTPRequestConfig{}, errors.New("HTTP request Generic Credential Type requires Header Auth")
	}
	if authMode == "none" && (credentialID != "" || genericAuthType != "") {
		return studioHTTPRequestConfig{}, errors.New("HTTP request credential requires credential authentication mode")
	}

	options, err := parseStudioHTTPRequestOptions(raw["options"])
	if err != nil {
		return studioHTTPRequestConfig{}, err
	}
	config := studioHTTPRequestConfig{
		Method: method, URL: rawURL, Headers: headers, Body: body,
		QueryParameters: query, AuthMode: authMode, GenericAuthType: genericAuthType,
		CredentialID: credentialID, Options: options,
	}
	if err := validateStudioHTTPRequestExpressionSyntax(config); err != nil {
		return studioHTTPRequestConfig{}, err
	}
	return config, nil
}

func parseStudioHTTPQueryParameters(raw any) ([]studioHTTPQueryParameter, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) > maxStudioHTTPQueryParameters {
		return nil, errors.New("HTTP request query parameters are invalid")
	}
	result := make([]studioHTTPQueryParameter, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			return nil, errors.New("HTTP request query parameters are invalid")
		}
		name, nameOK := record["name"].(string)
		value, valueOK := record["value"].(string)
		name = strings.TrimSpace(name)
		if !nameOK || !valueOK || name == "" || len(name) > maxStudioHTTPQueryNameBytes || len(value) > maxStudioHTTPQueryValueBytes || strings.ContainsAny(name+value, "\x00\r\n") {
			return nil, errors.New("HTTP request query parameters are invalid")
		}
		result = append(result, studioHTTPQueryParameter{Name: name, Value: value})
	}
	return result, nil
}

func parseStudioHTTPRequestOptions(raw any) (studioHTTPRequestOptions, error) {
	options := defaultStudioHTTPRequestOptions()
	if raw == nil {
		return options, nil
	}
	record, ok := raw.(map[string]any)
	if !ok {
		return options, errors.New("HTTP request options are invalid")
	}
	allowed := map[string]bool{
		"timeoutMs": true, "followRedirects": true, "maxRedirects": true,
		"responseFormat": true, "includeResponseHeaders": true, "ignoreHttpStatusErrors": true,
	}
	for key := range record {
		if !allowed[key] {
			return options, errors.New("HTTP request options contain an unsupported value")
		}
	}
	if value, exists := record["timeoutMs"]; exists {
		integer, ok := studioConfigInteger(value)
		if !ok || integer < 1000 || integer > 30000 {
			return options, errors.New("HTTP request timeout must be between 1,000 and 30,000 milliseconds")
		}
		options.TimeoutMS = integer
	}
	if value, exists := record["followRedirects"]; exists {
		var ok bool
		options.FollowRedirects, ok = value.(bool)
		if !ok {
			return options, errors.New("HTTP request redirect option is invalid")
		}
	}
	if value, exists := record["maxRedirects"]; exists {
		integer, ok := studioConfigInteger(value)
		if !ok || integer < 0 || integer > maxStudioHTTPRedirects {
			return options, errors.New("HTTP request redirect limit is invalid")
		}
		options.MaxRedirects = integer
	}
	if value, exists := record["responseFormat"]; exists {
		format, ok := value.(string)
		if !ok || (format != "auto" && format != "json" && format != "text") {
			return options, errors.New("HTTP request response format is invalid")
		}
		options.ResponseFormat = format
	}
	if value, exists := record["includeResponseHeaders"]; exists {
		var ok bool
		options.IncludeResponseHeaders, ok = value.(bool)
		if !ok {
			return options, errors.New("HTTP request response header option is invalid")
		}
	}
	if value, exists := record["ignoreHttpStatusErrors"]; exists {
		var ok bool
		options.IgnoreHTTPStatusErrors, ok = value.(bool)
		if !ok {
			return options, errors.New("HTTP request status option is invalid")
		}
	}
	return options, nil
}

func studioConfigInteger(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int64:
		return int(number), int64(int(number)) == number
	case float64:
		integer := int(number)
		return integer, float64(integer) == number
	default:
		return 0, false
	}
}

func studioCredentialMatchesGenericAuth(genericAuthType, credentialType string) bool {
	return genericAuthType == "headerAuth" && credentialType == "header"
}

func applyStudioCredential(request *http.Request, credential *studioResolvedCredential) error {
	if credential == nil {
		return nil
	}
	switch credential.Type {
	case "bearer":
		token := credential.Data["token"]
		if token == "" {
			return errors.New("bearer credential is invalid")
		}
		if request.Header.Get("Authorization") != "" {
			return errors.New("bearer credential conflicts with a static header")
		}
		request.Header.Set("Authorization", "Bearer "+token)
	case "basic":
		username, password := credential.Data["username"], credential.Data["password"]
		if username == "" || password == "" {
			return errors.New("basic credential is invalid")
		}
		if request.Header.Get("Authorization") != "" {
			return errors.New("basic credential conflicts with a static header")
		}
		request.SetBasicAuth(username, password)
	case "header":
		name, value := credential.Data["name"], credential.Data["value"]
		if name == "" || value == "" || validateStudioCredentialHeaders(map[string]string{name: value}) != nil {
			return errors.New("header credential is invalid")
		}
		if request.Header.Get(name) != "" {
			return errors.New("header credential conflicts with a static header")
		}
		request.Header.Set(name, value)
	case "query":
		name, value := strings.TrimSpace(credential.Data["name"]), credential.Data["value"]
		if name == "" || value == "" || len(name) > maxStudioHTTPQueryNameBytes || len(value) > maxStudioHTTPQueryValueBytes || strings.ContainsAny(name+value, "\x00\r\n") {
			return errors.New("query credential is invalid")
		}
		query := request.URL.Query()
		if query.Has(name) {
			return errors.New("query credential conflicts with a static query parameter")
		}
		query.Set(name, value)
		request.URL.RawQuery = query.Encode()
	default:
		return errors.New("credential type is unsupported")
	}
	return nil
}

func parseStudioCurlCommand(command string) (studioCurlImportResult, error) {
	if len(command) == 0 || len(command) > maxStudioCurlCommandBytes {
		return studioCurlImportResult{}, errors.New("cURL command is invalid")
	}
	tokens, err := splitStudioShellWords(command)
	if err != nil || len(tokens) < 2 || tokens[0] != "curl" {
		return studioCurlImportResult{}, errors.New("cURL command is invalid")
	}
	result := studioCurlImportResult{Method: http.MethodGet, Headers: map[string]string{}, Warnings: []string{}}
	explicitMethod := false
	for index := 1; index < len(tokens); index++ {
		token := tokens[index]
		next := func() (string, error) {
			index++
			if index >= len(tokens) {
				return "", errors.New("cURL option is missing a value")
			}
			return tokens[index], nil
		}
		switch {
		case token == "-X" || token == "--request":
			value, valueErr := next()
			if valueErr != nil {
				return studioCurlImportResult{}, valueErr
			}
			result.Method = strings.ToUpper(value)
			explicitMethod = true
		case strings.HasPrefix(token, "--request="):
			result.Method = strings.ToUpper(strings.TrimPrefix(token, "--request="))
			explicitMethod = true
		case token == "-H" || token == "--header":
			value, valueErr := next()
			if valueErr != nil {
				return studioCurlImportResult{}, valueErr
			}
			if err := addStudioCurlHeader(&result, value); err != nil {
				return studioCurlImportResult{}, err
			}
		case token == "-d" || token == "--data" || token == "--data-raw" || token == "--data-binary":
			if _, valueErr := next(); valueErr != nil {
				return studioCurlImportResult{}, valueErr
			}
			result.Warnings = append(result.Warnings, "Request body was removed because imported body content cannot be proven secret-free. Add a non-secret body manually.")
			if !explicitMethod {
				result.Method = http.MethodPost
			}
		case token == "-u" || token == "--user":
			if _, valueErr := next(); valueErr != nil {
				return studioCurlImportResult{}, valueErr
			}
			result.Warnings = append(result.Warnings, "Inline authentication was removed. Select a saved credential instead.")
		case token == "--url":
			value, valueErr := next()
			if valueErr != nil {
				return studioCurlImportResult{}, valueErr
			}
			result.URL = value
		case strings.HasPrefix(token, "--url="):
			result.URL = strings.TrimPrefix(token, "--url=")
		case token == "--compressed" || token == "-s" || token == "--silent" || token == "-L" || token == "--location":
			// Supported execution defaults make these flags unnecessary.
		case strings.HasPrefix(token, "-"):
			return studioCurlImportResult{}, fmt.Errorf("unsupported cURL option %q", token)
		default:
			if result.URL != "" {
				return studioCurlImportResult{}, errors.New("cURL command contains multiple URLs")
			}
			result.URL = token
		}
	}
	if !map[string]bool{http.MethodGet: true, http.MethodPost: true, http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true}[result.Method] {
		return studioCurlImportResult{}, errors.New("cURL method is unsupported")
	}
	parsed, err := validateStudioHTTPRequestURL(result.URL)
	if err != nil {
		return studioCurlImportResult{}, err
	}
	for name, values := range parsed.Query() {
		if isStudioSecretFieldName(name) {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Sensitive query parameter %q was removed. Select a saved credential instead.", name))
			continue
		}
		for _, value := range values {
			result.QueryParameters = append(result.QueryParameters, studioHTTPQueryParameter{Name: name, Value: value})
		}
	}
	parsed.RawQuery = ""
	result.URL = parsed.String()
	if err := validateStudioHTTPHeaders(result.Headers); err != nil {
		return studioCurlImportResult{}, err
	}
	if err := validateStudioHTTPRequestBody(result.Body); err != nil {
		return studioCurlImportResult{}, err
	}
	if err := validateStudioHTTPRequestSecretFreeConfig(map[string]any{"body": result.Body}); err != nil {
		return studioCurlImportResult{}, errors.New("cURL body contains credential material; create a saved credential instead")
	}
	return result, nil
}

func addStudioCurlHeader(result *studioCurlImportResult, raw string) error {
	name, value, found := strings.Cut(raw, ":")
	name, value = strings.TrimSpace(name), strings.TrimSpace(value)
	if !found || name == "" {
		return errors.New("cURL header is invalid")
	}
	if !strings.EqualFold(name, "Content-Type") {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Header %q was removed because imported header values cannot be proven secret-free. Add a non-secret header manually.", name))
		return nil
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || mediaType == "" {
		return errors.New("cURL Content-Type header is invalid")
	}
	if err := validateStudioHTTPHeaders(map[string]string{"Content-Type": mediaType}); err != nil {
		return err
	}
	result.Headers["Content-Type"] = mediaType
	return nil
}

func splitStudioShellWords(command string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	for _, char := range command {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if unicode.IsSpace(char) {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, errors.New("cURL command contains an unterminated quote or escape")
	}
	flush()
	if len(tokens) > 256 {
		return nil, errors.New("cURL command contains too many arguments")
	}
	return tokens, nil
}

func appendStudioQueryParameters(parsed *url.URL, parameters []studioHTTPQueryParameter) {
	query := parsed.Query()
	for _, parameter := range parameters {
		query.Add(parameter.Name, parameter.Value)
	}
	parsed.RawQuery = query.Encode()
}
