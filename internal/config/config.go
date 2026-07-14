package config

import "github.com/zeromicro/go-zero/rest"

type Config struct {
	rest.RestConf

	CorsOrigins []string `json:",optional"`

	SiteURL string `json:",optional"`

	SupabaseURL            string `json:",optional"`
	SupabasePublishableKey string `json:",optional"`
	SupabaseServiceRoleKey string `json:",optional"`

	RedisURL               string `json:",optional"`
	ArticleCacheTTLSeconds int    `json:",optional"`
	TrustProxy             bool   `json:",optional"`

	ContactWebhookURL    string `json:",optional"`
	ContactWebhookSecret string `json:",optional"`

	AdminApiToken                 string `json:",optional"`
	InternalApiToken              string `json:",optional"`
	StudioCredentialEncryptionKey string `json:",optional"`

	AiProvider string `json:",optional"`
	AiApiKey   string `json:",optional"`

	OllamaBaseURL       string `json:",optional"`
	OllamaModel         string `json:",optional"`
	OllamaAllowedModels string `json:",optional"`
	AISkillsDir         string `json:",optional"`

	PortfolioChatVisitorSecret     string `json:",optional"`
	PortfolioChatSessionTTLHours   int    `json:",optional"`
	PortfolioChatMaxStoredMessages int    `json:",optional"`
}
