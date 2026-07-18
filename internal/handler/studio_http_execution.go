package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/svc"
)

type studioNodeExecutionError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *studioNodeExecutionError) Error() string { return e.Message }

func executeStudioHTTPRequestNode(ctx context.Context, service *svc.ServiceContext, workflowID string, node model.StudioWorkflowNode, input []map[string]any) ([]map[string]any, *studioNodeExecutionError) {
	requestConfig, err := parseStudioHTTPRequestConfig(node.Config)
	if err != nil {
		return nil, &studioNodeExecutionError{Code: "invalid_config", Message: err.Error(), HTTPStatus: http.StatusBadRequest}
	}
	requestConfig, err = resolveStudioHTTPRequestExpressions(requestConfig, input)
	if err != nil {
		return nil, &studioNodeExecutionError{Code: "expression_error", Message: err.Error(), HTTPStatus: http.StatusBadRequest}
	}
	parsedURL, err := validateStudioHTTPRequestURL(requestConfig.URL)
	if err != nil {
		return nil, studioExecutionHTTPError(err)
	}
	appendStudioQueryParameters(parsedURL, requestConfig.QueryParameters)

	var requestBody io.Reader
	if requestConfig.Body != "" {
		requestBody = strings.NewReader(requestConfig.Body)
	}
	request, err := http.NewRequestWithContext(ctx, requestConfig.Method, parsedURL.String(), requestBody)
	if err != nil {
		return nil, &studioNodeExecutionError{Code: "invalid_request", Message: "Invalid HTTP request.", HTTPStatus: http.StatusBadRequest}
	}
	for name, value := range requestConfig.Headers {
		request.Header.Set(name, value)
	}

	credentialSecrets := []string{}
	if requestConfig.AuthMode == "credential" {
		if service == nil || service.Studio == nil || service.StudioCredentialCipher == nil {
			return nil, &studioNodeExecutionError{Code: "credential_unavailable", Message: "HTTP request credential is unavailable.", HTTPStatus: http.StatusConflict}
		}
		record, findErr := service.Studio.FindCredential(ctx, requestConfig.CredentialID)
		if findErr != nil {
			return nil, &studioNodeExecutionError{Code: "credential_unavailable", Message: "Unable to load HTTP request credential.", HTTPStatus: http.StatusConflict}
		}
		if record == nil || record.Status != "active" || !studioCredentialMatchesGenericAuth(requestConfig.GenericAuthType, record.Type) {
			return nil, &studioNodeExecutionError{Code: "credential_unavailable", Message: "HTTP request credential is unavailable.", HTTPStatus: http.StatusConflict}
		}
		data, decryptErr := service.StudioCredentialCipher.DecryptFor(studioCredentialCipherScope(record.ID, record.Type), record.EncryptedData)
		if decryptErr != nil {
			return nil, &studioNodeExecutionError{Code: "credential_unavailable", Message: "HTTP request credential is unavailable.", HTTPStatus: http.StatusConflict}
		}
		credentialSecrets = studioCredentialSecretValues(data)
		if err := applyStudioCredential(request, &studioResolvedCredential{Type: record.Type, Data: data}); err != nil {
			return nil, &studioNodeExecutionError{Code: "credential_invalid", Message: "HTTP request credential is invalid.", HTTPStatus: http.StatusConflict}
		}
	}

	httpClient := studioSafeHTTPClient
	if requestConfig.Options != defaultStudioHTTPRequestOptions() {
		customClient := newStudioSafeHTTPClientWithOptions(requestConfig.Options)
		defer customClient.CloseIdleConnections()
		httpClient = customClient
	}
	responseValue, err := httpClient.Do(request)
	if err != nil {
		observability.ErrorType(ctx, "studio.http_request.failed", "Studio graph HTTP request failed", err)
		return nil, studioExecutionHTTPError(err)
	}
	defer responseValue.Body.Close()

	responseBody, err := readStudioHTTPResponseBody(responseValue.Body)
	if err != nil {
		if errors.Is(err, errStudioHTTPResponseTooLarge) {
			return nil, &studioNodeExecutionError{Code: "response_too_large", Message: errStudioHTTPResponseTooLarge.Error(), HTTPStatus: http.StatusBadGateway}
		}
		return nil, &studioNodeExecutionError{Code: "response_read", Message: "Unable to read the HTTP response.", HTTPStatus: http.StatusBadGateway}
	}
	if !requestConfig.Options.IgnoreHTTPStatusErrors && responseValue.StatusCode >= http.StatusBadRequest {
		return nil, &studioNodeExecutionError{Code: "http_status", Message: "HTTP request returned an error status.", HTTPStatus: http.StatusBadGateway}
	}

	var parsedBody any
	switch requestConfig.Options.ResponseFormat {
	case "text":
		parsedBody = string(responseBody)
	case "json":
		if json.Unmarshal(responseBody, &parsedBody) != nil {
			return nil, &studioNodeExecutionError{Code: "invalid_json", Message: "HTTP response was not valid JSON.", HTTPStatus: http.StatusBadGateway}
		}
	default:
		if json.Unmarshal(responseBody, &parsedBody) != nil {
			parsedBody = string(responseBody)
		}
	}
	parsedBody = redactStudioCredentialValues(parsedBody, credentialSecrets)
	redactedStatus, _ := redactStudioCredentialValues(responseValue.Status, credentialSecrets).(string)
	outputJSON := map[string]any{"statusCode": responseValue.StatusCode, "status": redactedStatus, "body": parsedBody}
	if requestConfig.Options.IncludeResponseHeaders {
		outputJSON["headers"] = filterStudioHTTPResponseHeaders(responseValue.Header, credentialSecrets)
	}
	return sanitizeStudioExecutionItems([]map[string]any{{"json": outputJSON}}), nil
}

func studioExecutionHTTPError(err error) *studioNodeExecutionError {
	switch {
	case errors.Is(err, errStudioHTTPDestinationBlocked):
		return &studioNodeExecutionError{Code: "destination_blocked", Message: errStudioHTTPDestinationBlocked.Error(), HTTPStatus: http.StatusBadRequest}
	case errors.Is(err, errStudioHTTPResolutionFailed):
		return &studioNodeExecutionError{Code: "dns_resolution", Message: errStudioHTTPResolutionFailed.Error(), HTTPStatus: http.StatusBadGateway}
	case errors.Is(err, context.DeadlineExceeded):
		return &studioNodeExecutionError{Code: "timeout", Message: "HTTP request timed out.", HTTPStatus: http.StatusGatewayTimeout}
	default:
		return &studioNodeExecutionError{Code: "transport", Message: "HTTP request failed.", HTTPStatus: http.StatusBadGateway}
	}
}
