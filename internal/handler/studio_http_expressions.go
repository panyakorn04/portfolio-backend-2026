package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var studioJSONPathExpression = regexp.MustCompile(`\{\{\$json\.([A-Za-z_][A-Za-z0-9_-]*(?:\.[A-Za-z_][A-Za-z0-9_-]*)*)\}\}`)

func validateStudioHTTPRequestExpressionSyntax(config studioHTTPRequestConfig) error {
	if studioStringHasExpression(config.URL) {
		parsed, err := url.Parse(config.URL)
		if err != nil {
			return errors.New("HTTP request URL expression is invalid")
		}
		if studioStringHasExpression(parsed.RawQuery) {
			return errors.New("HTTP request URL query expressions are unsupported; use Query Parameters")
		}
		if err := validateStudioExpressionTemplate(parsed.Path); err != nil {
			return fmt.Errorf("HTTP request URL expression is invalid: %w", err)
		}
	}
	for _, value := range config.Headers {
		if studioStringHasExpression(value) {
			if err := validateStudioExpressionTemplate(value); err != nil {
				return fmt.Errorf("HTTP request header expression is invalid: %w", err)
			}
		}
	}
	for _, parameter := range config.QueryParameters {
		if studioStringHasExpression(parameter.Value) {
			if err := validateStudioExpressionTemplate(parameter.Value); err != nil {
				return fmt.Errorf("HTTP request query expression is invalid: %w", err)
			}
		}
	}
	if studioStringHasExpression(config.Body) {
		var decoded any
		if err := json.Unmarshal([]byte(config.Body), &decoded); err != nil {
			return errors.New("HTTP request body expressions require a valid JSON body")
		}
		if err := validateStudioJSONBodyExpressionSyntax(decoded); err != nil {
			return fmt.Errorf("HTTP request body expression is invalid: %w", err)
		}
	}
	return nil
}

func validateStudioJSONBodyExpressionSyntax(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if studioStringHasExpression(key) {
				return errors.New("expressions are allowed only in JSON body values")
			}
			if err := validateStudioJSONBodyExpressionSyntax(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := validateStudioJSONBodyExpressionSyntax(child); err != nil {
				return err
			}
		}
	case string:
		if studioStringHasExpression(typed) {
			return validateStudioExpressionTemplate(typed)
		}
	}
	return nil
}

func resolveStudioHTTPRequestExpressions(config studioHTTPRequestConfig, items []map[string]any) (studioHTTPRequestConfig, error) {
	resolved := config
	resolved.Headers = make(map[string]string, len(config.Headers))
	for name, value := range config.Headers {
		resolved.Headers[name] = value
	}
	resolved.QueryParameters = append([]studioHTTPQueryParameter(nil), config.QueryParameters...)

	if !studioHTTPRequestConfigHasExpressions(config) {
		return resolved, nil
	}
	root, err := studioFirstIncomingJSONItem(items)
	if err != nil {
		return studioHTTPRequestConfig{}, err
	}
	resolved.URL, err = resolveStudioURLPathExpressionTemplate(config.URL, root)
	if err != nil {
		return studioHTTPRequestConfig{}, fmt.Errorf("HTTP request URL expression is invalid: %w", err)
	}
	if len(resolved.URL) > maxStudioHTTPURLBytes {
		return studioHTTPRequestConfig{}, errors.New("HTTP request URL is too long after expression mapping")
	}
	for name, value := range resolved.Headers {
		mapped, mapErr := resolveStudioScalarExpressionTemplate(value, root)
		if mapErr != nil {
			return studioHTTPRequestConfig{}, fmt.Errorf("HTTP request header expression is invalid: %w", mapErr)
		}
		resolved.Headers[name] = mapped
	}
	if err := validateStudioHTTPHeaders(resolved.Headers); err != nil {
		return studioHTTPRequestConfig{}, err
	}
	for index := range resolved.QueryParameters {
		mapped, mapErr := resolveStudioScalarExpressionTemplate(resolved.QueryParameters[index].Value, root)
		if mapErr != nil {
			return studioHTTPRequestConfig{}, fmt.Errorf("HTTP request query expression is invalid: %w", mapErr)
		}
		if len(mapped) > maxStudioHTTPQueryValueBytes || strings.ContainsAny(mapped, "\x00\r\n") {
			return studioHTTPRequestConfig{}, errors.New("HTTP request query parameter is invalid after expression mapping")
		}
		resolved.QueryParameters[index].Value = mapped
	}
	if studioStringHasExpression(config.Body) {
		resolved.Body, err = resolveStudioJSONBodyExpressions(config.Body, root)
		if err != nil {
			return studioHTTPRequestConfig{}, err
		}
	}
	return resolved, nil
}

func studioHTTPRequestConfigHasExpressions(config studioHTTPRequestConfig) bool {
	if studioStringHasExpression(config.URL) || studioStringHasExpression(config.Body) {
		return true
	}
	for _, value := range config.Headers {
		if studioStringHasExpression(value) {
			return true
		}
	}
	for _, parameter := range config.QueryParameters {
		if studioStringHasExpression(parameter.Value) {
			return true
		}
	}
	return false
}

func studioStringHasExpression(value string) bool {
	return strings.Contains(value, "{{") || strings.Contains(value, "}}")
}

func studioFirstIncomingJSONItem(items []map[string]any) (map[string]any, error) {
	if len(items) == 0 {
		return nil, errors.New("HTTP request expressions require an incoming JSON item; run the previous nodes first")
	}
	root, ok := items[0]["json"].(map[string]any)
	if !ok {
		return nil, errors.New("HTTP request expressions require the first incoming item to contain a JSON object")
	}
	return root, nil
}

func validateStudioExpressionTemplate(value string) error {
	remainder := studioJSONPathExpression.ReplaceAllString(value, "")
	if strings.Contains(remainder, "{{") || strings.Contains(remainder, "}}") {
		return errors.New("unsupported expression; only {{$json.path}} is allowed")
	}
	return nil
}

func resolveStudioURLPathExpressionTemplate(template string, root map[string]any) (string, error) {
	if !studioStringHasExpression(template) {
		return template, nil
	}
	parsed, err := url.Parse(template)
	if err != nil {
		return "", errors.New("URL could not be parsed")
	}
	if studioStringHasExpression(parsed.RawQuery) {
		return "", errors.New("URL query expressions are unsupported; use Query Parameters")
	}
	segments := strings.Split(parsed.Path, "/")
	escapedSegments := make([]string, len(segments))
	for index, segment := range segments {
		if !studioStringHasExpression(segment) {
			escapedSegments[index] = url.PathEscape(segment)
			continue
		}
		if err := validateStudioExpressionTemplate(segment); err != nil {
			return "", err
		}
		var resolveErr error
		mapped := studioJSONPathExpression.ReplaceAllStringFunc(segment, func(expression string) string {
			if resolveErr != nil {
				return ""
			}
			matches := studioJSONPathExpression.FindStringSubmatch(expression)
			value, lookupErr := resolveStudioJSONPath(root, matches[1])
			if lookupErr != nil {
				resolveErr = lookupErr
				return ""
			}
			scalar, scalarErr := studioExpressionScalarString(value)
			if scalarErr != nil {
				resolveErr = scalarErr
				return ""
			}
			if scalar == "." || scalar == ".." {
				resolveErr = errors.New("URL path expression values must not be traversal segments")
				return ""
			}
			if strings.ContainsAny(scalar, "/\\") {
				resolveErr = errors.New("URL path expression values must not contain slashes")
				return ""
			}
			return scalar
		})
		if resolveErr != nil {
			return "", resolveErr
		}
		escapedSegments[index] = encodeStudioURLPathSegment(mapped)
	}
	rawPath := strings.Join(escapedSegments, "/")
	decodedPath, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", errors.New("URL path expression produced an invalid path")
	}
	parsed.Path = decodedPath
	parsed.RawPath = rawPath
	return parsed.String(), nil
}

func encodeStudioURLPathSegment(value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var encoded strings.Builder
	encoded.Grow(len(value))
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("-._~", rune(char)) {
			encoded.WriteByte(char)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexadecimal[char>>4])
		encoded.WriteByte(hexadecimal[char&15])
	}
	return encoded.String()
}

func resolveStudioScalarExpressionTemplate(template string, root map[string]any) (string, error) {
	if !studioStringHasExpression(template) {
		return template, nil
	}
	if err := validateStudioExpressionTemplate(template); err != nil {
		return "", err
	}
	var resolveErr error
	resolved := studioJSONPathExpression.ReplaceAllStringFunc(template, func(expression string) string {
		if resolveErr != nil {
			return ""
		}
		matches := studioJSONPathExpression.FindStringSubmatch(expression)
		value, err := resolveStudioJSONPath(root, matches[1])
		if err != nil {
			resolveErr = err
			return ""
		}
		scalar, err := studioExpressionScalarString(value)
		if err != nil {
			resolveErr = err
			return ""
		}
		return scalar
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	return resolved, nil
}

func resolveStudioJSONPath(root map[string]any, path string) (any, error) {
	var current any = root
	for _, segment := range strings.Split(path, ".") {
		record, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("JSON path %q was not found", path)
		}
		current, ok = record[segment]
		if !ok {
			return nil, fmt.Errorf("JSON path %q was not found", path)
		}
	}
	if current == nil {
		return nil, fmt.Errorf("JSON path %q resolved to null", path)
	}
	return current, nil
}

func studioExpressionScalarString(value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case bool, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, json.Number:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", errors.New("JSON path did not resolve to a supported scalar value")
		}
		return string(encoded), nil
	default:
		return "", errors.New("JSON path must resolve to a string, number, or boolean scalar")
	}
}

func resolveStudioJSONBodyExpressions(body string, root map[string]any) (string, error) {
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		return "", errors.New("HTTP request body expressions require a valid JSON body")
	}
	mapped, err := resolveStudioJSONBodyValue(decoded, root)
	if err != nil {
		return "", fmt.Errorf("HTTP request body expression is invalid: %w", err)
	}
	if containsStudioSecretJSONKey(mapped) {
		return "", errors.New("HTTP request mapped body contains a secret field name")
	}
	encoded, err := json.Marshal(mapped)
	if err != nil {
		return "", errors.New("HTTP request mapped body is invalid")
	}
	resolved := string(encoded)
	if err := validateStudioHTTPRequestBody(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func resolveStudioJSONBodyValue(value any, root map[string]any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		mapped := make(map[string]any, len(typed))
		for key, child := range typed {
			if studioStringHasExpression(key) {
				return nil, errors.New("expressions are allowed only in JSON body values")
			}
			resolved, err := resolveStudioJSONBodyValue(child, root)
			if err != nil {
				return nil, err
			}
			mapped[key] = resolved
		}
		return mapped, nil
	case []any:
		mapped := make([]any, len(typed))
		for index, child := range typed {
			resolved, err := resolveStudioJSONBodyValue(child, root)
			if err != nil {
				return nil, err
			}
			mapped[index] = resolved
		}
		return mapped, nil
	case string:
		if !studioStringHasExpression(typed) {
			return typed, nil
		}
		if err := validateStudioExpressionTemplate(typed); err != nil {
			return nil, err
		}
		if studioJSONPathExpression.MatchString(typed) && studioJSONPathExpression.FindString(typed) == typed {
			matches := studioJSONPathExpression.FindStringSubmatch(typed)
			return resolveStudioJSONPath(root, matches[1])
		}
		return resolveStudioScalarExpressionTemplate(typed, root)
	default:
		return typed, nil
	}
}
