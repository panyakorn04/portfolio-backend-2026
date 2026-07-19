package svc

import (
	"strings"
	"testing"

	"portfolio-backend/internal/config"

	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/rest"
)

func TestNewServiceContextRequiresPortfolioVisitorSecretInProduction(t *testing.T) {
	for _, secret := range []string{"", "   "} {
		_, err := NewServiceContext(config.Config{
			RestConf:                   restConfForTest("pro"),
			PortfolioChatVisitorSecret: secret,
		})
		if err == nil || !strings.Contains(err.Error(), "PORTFOLIO_CHAT_VISITOR_SECRET") {
			t.Fatalf("NewServiceContext() error = %v, want missing visitor-secret error", err)
		}
	}
}

func TestNewServiceContextAllowsExplicitDevelopmentFallback(t *testing.T) {
	context, err := NewServiceContext(config.Config{RestConf: restConfForTest("dev")})
	if err != nil {
		t.Fatalf("NewServiceContext() error = %v", err)
	}
	context.Close()
}

func restConfForTest(mode string) rest.RestConf {
	return rest.RestConf{ServiceConf: service.ServiceConf{Mode: mode}}
}
