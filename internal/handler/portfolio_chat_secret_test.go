package handler

import (
	"testing"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/svc"
)

func TestPortfolioChatVisitorSecretNeverReusesProductionCredentials(t *testing.T) {
	service := &svc.ServiceContext{Config: config.Config{
		SupabaseServiceRoleKey: "service-role-value",
		AdminApiToken:          "admin-token-value",
	}}

	secret := portfolioChatVisitorSecret(service)
	if secret != "portfolio-chat-development-secret" {
		t.Fatalf("fallback secret = %q, want explicit development fallback", secret)
	}
}
