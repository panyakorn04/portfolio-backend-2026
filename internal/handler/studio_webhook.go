package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

const maxStudioWebhookBodyBytes = 1 << 20

func validStudioWebhookSigningKey(secret, credentialKey string) bool {
	return len(secret) >= 32 && !hmac.Equal([]byte(secret), []byte(credentialKey))
}

func studioWebhookToken(secret, workflowID, nodeID, version string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("studio-webhook:v2:" + workflowID + ":" + nodeID + ":" + version))
	return hex.EncodeToString(mac.Sum(nil))
}

func effectiveStudioWebhookTokenVersion(workflow *model.StudioWorkflow, node *model.StudioWorkflowNode) string {
	if version := studioWebhookTokenVersion(node); version != "" {
		return version
	}
	if workflow == nil || node == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(workflow.UpdatedAt.UTC().Format(time.RFC3339Nano) + ":" + node.ID))
	return hex.EncodeToString(sum[:16])
}

func newStudioWebhookTokenVersion() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return ""
	}
	return hex.EncodeToString(value[:])
}

func studioWebhookTokenVersion(node *model.StudioWorkflowNode) string {
	if node == nil {
		return ""
	}
	version, _ := node.Config["webhookTokenVersion"].(string)
	return strings.TrimSpace(version)
}

func AdminStudioWebhookURLHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		access, ok := requireAdmin(w, r, service)
		if !ok || !assertRole(w, access, []string{"admin", "editor"}) || !requireStudioDB(w, service) {
			return
		}
		workflowID := strings.TrimSpace(pathParam(r, "id"))
		nodeID := strings.TrimSpace(pathParam(r, "nodeId"))
		workflow, err := service.Studio.FindWorkflow(r.Context(), workflowID)
		if err != nil {
			response.Error(w, http.StatusInternalServerError, "Unable to load workflow.")
			return
		}
		node := findStudioWorkflowNode(workflow, nodeID)
		if workflow == nil || node == nil || node.Type != "webhook" {
			response.Error(w, http.StatusNotFound, "Webhook node was not found.")
			return
		}
		secret := service.Config.StudioWebhookSigningKey
		version := effectiveStudioWebhookTokenVersion(workflow, node)
		if !validStudioWebhookSigningKey(secret, service.Config.StudioCredentialEncryptionKey) || version == "" {
			response.Error(w, http.StatusServiceUnavailable, "Webhook signing is not configured.")
			return
		}
		token := studioWebhookToken(secret, workflowID, nodeID, version)
		path := "/api/studio/webhooks/" + url.PathEscape(workflowID) + "/" + url.PathEscape(nodeID)
		response.Ok(w, http.StatusOK, map[string]string{
			"path": path, "header": "X-Studio-Webhook-Token", "token": token,
			"versionHeader": "X-Studio-Webhook-Version", "version": version,
		})
	}
}

func parseStudioWebhookBody(contentType string, body []byte) (any, error) {
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, "application/json") {
		var parsed any
		if json.Unmarshal(body, &parsed) != nil {
			return nil, errors.New("Webhook JSON body is invalid.")
		}
		return parsed, nil
	}
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, errors.New("Webhook form body is invalid.")
		}
		form := map[string]any{}
		for key, entries := range values {
			if isStudioSecretFieldName(key) {
				form[key] = "[REDACTED]"
			} else {
				form[key] = append([]string(nil), entries...)
			}
		}
		return form, nil
	}
	return redactStudioPersistedString(string(body)), nil
}

func StudioWebhookHandler(service *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil || service.Studio == nil || !validStudioWebhookSigningKey(service.Config.StudioWebhookSigningKey, service.Config.StudioCredentialEncryptionKey) {
			response.Error(w, http.StatusServiceUnavailable, "Webhook execution is not configured.")
			return
		}
		workflowID := strings.TrimSpace(pathParam(r, "id"))
		nodeID := strings.TrimSpace(pathParam(r, "nodeId"))
		providedToken := strings.TrimSpace(r.Header.Get("X-Studio-Webhook-Token"))
		providedVersion := strings.TrimSpace(r.Header.Get("X-Studio-Webhook-Version"))
		versionBytes, versionErr := hex.DecodeString(providedVersion)
		expectedToken := studioWebhookToken(service.Config.StudioWebhookSigningKey, workflowID, nodeID, providedVersion)
		providedBytes, decodeErr := hex.DecodeString(providedToken)
		expectedBytes, _ := hex.DecodeString(expectedToken)
		if versionErr != nil || len(versionBytes) != 16 || decodeErr != nil || !hmac.Equal(providedBytes, expectedBytes) {
			response.Error(w, http.StatusNotFound, "Webhook was not found.")
			return
		}
		workflow, err := service.Studio.FindWorkflow(r.Context(), workflowID)
		if err != nil {
			response.Error(w, http.StatusServiceUnavailable, "Webhook is temporarily unavailable.")
			return
		}
		node := findStudioWorkflowNode(workflow, nodeID)
		if workflow == nil || workflow.Status != "active" || node == nil || node.Type != "webhook" || node.Kind != "trigger" {
			response.Error(w, http.StatusNotFound, "Webhook was not found.")
			return
		}
		version := effectiveStudioWebhookTokenVersion(workflow, node)
		if version == "" || !hmac.Equal([]byte(providedVersion), []byte(version)) {
			response.Error(w, http.StatusNotFound, "Webhook was not found.")
			return
		}
		enabled, _ := node.Config["enabled"].(bool)
		method, _ := node.Config["method"].(string)
		if !enabled || r.Method != method {
			response.Error(w, http.StatusMethodNotAllowed, "Webhook method is not allowed.")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxStudioWebhookBodyBytes))
		if err != nil {
			response.Error(w, http.StatusRequestEntityTooLarge, "Webhook body is too large.")
			return
		}
		parsedBody, parseErr := parseStudioWebhookBody(r.Header.Get("Content-Type"), body)
		if parseErr != nil {
			response.Error(w, http.StatusBadRequest, parseErr.Error())
			return
		}
		query := map[string]any{}
		for key, values := range r.URL.Query() {
			if key == "token" || isStudioSecretFieldName(key) {
				continue
			}
			query[key] = append([]string(nil), values...)
		}
		headers := map[string]any{}
		for _, name := range []string{"Content-Type", "User-Agent", "X-Request-ID"} {
			if value := r.Header.Get(name); value != "" {
				headers[name] = value
			}
		}
		initial := sanitizeStudioExecutionItems([]map[string]any{{"json": map[string]any{
			"headers": headers, "query": query, "body": parsedBody,
		}}})
		payload := studioGraphExecutionPayload{TriggerNodeID: nodeID, Mode: "full", Input: initial}
		if deliveryID := strings.TrimSpace(r.Header.Get("X-Studio-Delivery-ID")); deliveryID != "" && len(deliveryID) <= 128 {
			payload.SourceKey = deliveryID
		}
		requestForContext := r.Clone(r.Context())
		item, message := enqueueStudioWorkflowExecution(requestForContext, service, workflow, payload, "webhook", "")
		if message != "" {
			response.Error(w, http.StatusConflict, "Webhook execution could not be queued.")
			return
		}
		response.Ok(w, http.StatusAccepted, map[string]string{"executionId": item.ID, "status": item.Status})
	}
}

func ensureStudioWebhookTokenVersions(definition *model.StudioWorkflowDefinition) bool {
	if definition == nil {
		return true
	}
	for index := range definition.Nodes {
		node := &definition.Nodes[index]
		if node.Type != "webhook" {
			continue
		}
		if node.Config == nil {
			node.Config = map[string]any{}
		}
		if authMode, _ := node.Config["authMode"].(string); authMode == "" || authMode == "none" {
			node.Config["authMode"] = "capability"
		}
		version := studioWebhookTokenVersion(node)
		decoded, err := hex.DecodeString(version)
		if err == nil && len(decoded) == 16 {
			continue
		}
		version = newStudioWebhookTokenVersion()
		if version == "" {
			return false
		}
		if node.Config == nil {
			node.Config = map[string]any{}
		}
		node.Config["webhookTokenVersion"] = version
	}
	return true
}

func findStudioWorkflowNode(workflow *model.StudioWorkflow, nodeID string) *model.StudioWorkflowNode {
	if workflow == nil || workflow.Definition == nil {
		return nil
	}
	for index := range workflow.Definition.Nodes {
		if workflow.Definition.Nodes[index].ID == nodeID {
			return &workflow.Definition.Nodes[index]
		}
	}
	return nil
}
