package svc

import (
	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"
)

type ServiceContext struct {
	Config config.Config

	Supabase *model.SupabaseREST

	Users            *model.UserModel
	Sessions         *model.AuthSessionModel
	Inquiries        *model.ContactInquiryModel
	Articles         *model.ArticleModel
	SupabaseArticles *model.SupabaseArticleClient
	HasDatabse       bool
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	svc := &ServiceContext{Config: c}

	key := c.SupabaseServiceRoleKey
	if key == "" {
		key = c.SupabasePublishableKey
	}

	svc.Supabase = model.NewSupabaseREST(c.SupabaseURL, key)
	svc.SupabaseArticles = model.NewSupabaseArticleClient(c.SupabaseURL, key)
	if svc.Supabase == nil {
		return svc, nil
	}

	svc.HasDatabse = true
	svc.Users = model.NewUserModel(svc.Supabase)
	svc.Sessions = model.NewAuthSessionModel(svc.Supabase)
	svc.Inquiries = model.NewContactInquiryModel(svc.Supabase)
	svc.Articles = model.NewArticleModel(svc.Supabase)

	return svc, nil
}

func (s *ServiceContext) Close() {}
