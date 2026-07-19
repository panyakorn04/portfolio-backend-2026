package config

import (
	"os"
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestRuntimeEnvironmentMappings(t *testing.T) {
	environment := map[string]string{
		"NEXT_PUBLIC_SITE_URL":                 "https://example.test",
		"NEXT_PUBLIC_SUPABASE_URL":             "https://project.supabase.test",
		"NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY": "publishable-test-value",
		"SUPABASE_SERVICE_ROLE_KEY":            "service-role-test-value",
		"REDIS_URL":                            "redis://127.0.0.1:6379/1",
		"ARTICLE_CACHE_TTL_SECONDS":            "321",
		"TRUST_PROXY":                          "true",
		"CONTACT_WEBHOOK_URL":                  "https://hooks.example.test/contact",
		"CONTACT_WEBHOOK_SECRET":               "contact-secret-test-value",
		"ADMIN_API_TOKEN":                      "admin-test-value",
		"INTERNAL_API_TOKEN":                   "internal-test-value",
		"STUDIO_CREDENTIAL_ENCRYPTION_KEY":     "studio-encryption-test-value",
		"STUDIO_WEBHOOK_SIGNING_KEY":           "studio-signing-test-value",
		"AI_PROVIDER":                          "stub",
		"AI_API_KEY":                           "ai-test-value",
		"OLLAMA_BASE_URL":                      "http://127.0.0.1:11434",
		"OLLAMA_MODEL":                         "test-model",
		"OLLAMA_ALLOWED_MODELS":                "test-model,other-model",
		"AI_SKILLS_DIR":                        "/tmp/ai-skills",
		"PORTFOLIO_CHAT_VISITOR_SECRET":        "visitor-test-value",
		"PORTFOLIO_CHAT_SESSION_TTL_HOURS":     "720",
		"PORTFOLIO_CHAT_MAX_STORED_MESSAGES":   "75",
	}
	for key, value := range environment {
		t.Setenv(key, value)
	}

	var loaded Config
	if err := conf.Load("../../etc/portfolio-api.yaml", &loaded, conf.UseEnv()); err != nil {
		t.Fatalf("conf.Load() error = %v", err)
	}
	if err := ApplyEnvironmentOverrides(&loaded, environmentLookup); err != nil {
		t.Fatalf("ApplyEnvironmentOverrides() error = %v", err)
	}

	if loaded.AiApiKey != environment["AI_API_KEY"] {
		t.Fatalf("AiApiKey = %q, want environment value", loaded.AiApiKey)
	}
	if loaded.PortfolioChatSessionTTLHours != 720 {
		t.Fatalf("PortfolioChatSessionTTLHours = %d, want 720", loaded.PortfolioChatSessionTTLHours)
	}
	if loaded.PortfolioChatMaxStoredMessages != 75 {
		t.Fatalf("PortfolioChatMaxStoredMessages = %d, want 75", loaded.PortfolioChatMaxStoredMessages)
	}
}

func TestRuntimeEnvironmentMappingsKeepDefaultsWhenAbsent(t *testing.T) {
	loaded := Config{
		PortfolioChatSessionTTLHours:   2160,
		PortfolioChatMaxStoredMessages: 100,
	}
	if err := ApplyEnvironmentOverrides(&loaded, func(string) (string, bool) { return "", false }); err != nil {
		t.Fatalf("ApplyEnvironmentOverrides() error = %v", err)
	}
	if loaded.PortfolioChatSessionTTLHours != 2160 || loaded.PortfolioChatMaxStoredMessages != 100 {
		t.Fatalf("defaults changed: %#v", loaded)
	}
}

func TestRuntimeEnvironmentMappingsRejectInvalidPositiveIntegers(t *testing.T) {
	loaded := Config{PortfolioChatSessionTTLHours: 2160}
	lookup := func(name string) (string, bool) {
		if name == "PORTFOLIO_CHAT_SESSION_TTL_HOURS" {
			return "0", true
		}
		return "", false
	}
	if err := ApplyEnvironmentOverrides(&loaded, lookup); err == nil {
		t.Fatal("ApplyEnvironmentOverrides() error = nil, want invalid positive integer")
	}
}

func environmentLookup(name string) (string, bool) {
	return os.LookupEnv(name)
}
