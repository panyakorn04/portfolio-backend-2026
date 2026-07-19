package svc

import (
	"errors"
	"strings"
	"time"

	"portfolio-backend/internal/cache"
	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
	"portfolio-backend/internal/security"
)

type ServiceContext struct {
	Config config.Config

	Supabase     *model.SupabaseREST
	Storage      *model.SupabaseStorage
	ArticleCache *cache.RedisCache

	Users                  *model.UserModel
	Sessions               *model.AuthSessionModel
	Inquiries              *model.ContactInquiryModel
	Articles               *model.ArticleModel
	SupabaseArticles       *model.SupabaseArticleClient
	PortfolioChatSessions  *model.PortfolioChatSessionModel
	PortfolioChatMessages  *model.PortfolioChatMessageModel
	Studio                 *model.StudioModel
	StudioCredentialCipher *security.CredentialCipher
	Ollama                 *model.OllamaClient
	AISkills               *model.AISkillProfileStore
	HasDatabse             bool
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	if c.Mode == "pro" && strings.TrimSpace(c.PortfolioChatVisitorSecret) == "" {
		return nil, errors.New("PORTFOLIO_CHAT_VISITOR_SECRET is required in production")
	}
	svc := &ServiceContext{Config: c}
	if c.StudioCredentialEncryptionKey != "" {
		credentialCipher, err := security.NewCredentialCipher(c.StudioCredentialEncryptionKey)
		if err != nil {
			return nil, err
		}
		svc.StudioCredentialCipher = credentialCipher
	}

	key := c.SupabaseServiceRoleKey
	if key == "" {
		key = c.SupabasePublishableKey
	}

	svc.Supabase = model.NewSupabaseREST(c.SupabaseURL, key)
	svc.Storage = model.NewSupabaseStorage(c.SupabaseURL, c.SupabaseServiceRoleKey)
	svc.SupabaseArticles = model.NewSupabaseArticleClient(c.SupabaseURL, key)
	svc.Ollama = model.NewOllamaClient(c.OllamaBaseURL, c.OllamaModel)
	svc.AISkills = model.NewAISkillProfileStore(c.AISkillsDir)

	articleCacheTTL := time.Duration(c.ArticleCacheTTLSeconds) * time.Second
	articleCache, err := cache.NewRedisCache(c.RedisURL, articleCacheTTL)
	if err != nil {
		return nil, err
	}
	svc.ArticleCache = articleCache

	if svc.Supabase == nil {
		return svc, nil
	}

	svc.HasDatabse = true
	svc.Users = model.NewUserModel(svc.Supabase)
	svc.Sessions = model.NewAuthSessionModel(svc.Supabase)
	svc.Inquiries = model.NewContactInquiryModel(svc.Supabase)
	svc.Articles = model.NewArticleModel(svc.Supabase)
	svc.PortfolioChatSessions = model.NewPortfolioChatSessionModel(svc.Supabase)
	svc.PortfolioChatMessages = model.NewPortfolioChatMessageModel(svc.Supabase)
	svc.Studio = model.NewStudioModel(svc.Supabase)

	return svc, nil
}

func (s *ServiceContext) Close() {
	if s.ArticleCache != nil {
		_ = s.ArticleCache.Close()
	}
}
