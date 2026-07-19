package config

import (
	"path/filepath"
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestProductionConfigUsesSanitizedApplicationRequestLogging(t *testing.T) {
	t.Setenv("ARTICLE_CACHE_TTL_SECONDS", "300")
	t.Setenv("TRUST_PROXY", "false")
	t.Setenv("PORTFOLIO_CHAT_SESSION_TTL_HOURS", "2160")
	t.Setenv("PORTFOLIO_CHAT_MAX_STORED_MESSAGES", "100")

	var cfg Config
	path := filepath.Join("..", "..", "etc", "portfolio-api.yaml")
	if err := conf.Load(path, &cfg, conf.UseEnv()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Middlewares.Log {
		t.Fatal("go-zero request logging must remain disabled because it can dump request secrets on 5xx")
	}
	if !cfg.Middlewares.Trace || !cfg.Middlewares.Prometheus || !cfg.Middlewares.Recover {
		t.Fatalf("required middleware disabled: %#v", cfg.Middlewares)
	}
	if cfg.Log.Mode != "console" || cfg.Log.Encoding != "json" || cfg.Log.Level != "info" {
		t.Fatalf("unexpected production log config: %#v", cfg.Log)
	}
}
