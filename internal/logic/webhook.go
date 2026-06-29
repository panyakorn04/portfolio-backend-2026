package logic

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"portfolio-backend/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

var webhookClient = &http.Client{Timeout: 10 * time.Second}

// SendContactWebhook delivers the payload to the configured webhook.
// Returns true only on a 2xx response. Failures are logged, not fatal.
func SendContactWebhook(svcCtx *svc.ServiceContext, payload any) bool {
	url := svcCtx.Config.ContactWebhookURL
	if url == "" {
		return false
	}

	body, err := json.Marshal(payload)
	if err != nil {
		logx.Errorf("[contact] webhook marshal failed: %v", err)
		return false
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logx.Errorf("[contact] webhook request build failed: %v", err)
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	if secret := svcCtx.Config.ContactWebhookSecret; secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := webhookClient.Do(req)
	if err != nil {
		logx.Errorf("[contact] webhook delivery failed: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		logx.Errorf("[contact] webhook delivery failed with status %d", resp.StatusCode)
		return false
	}
	return true
}
