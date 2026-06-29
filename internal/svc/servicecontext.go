package svc

import (
	"database/sql"

	"portfolio-backend/internal/config"
	"portfolio-backend/internal/model"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type ServiceContext struct {
	Config config.Config
	DB     *sql.DB

	Users            *model.UserModel
	Sessions         *model.AuthSessionModel
	Inquiries        *model.ContactInquiryModel
	Articles         *model.ArticleModel
	SupabaseArticles *model.SupabaseArticleClient
	HasDatabse       bool
}

func NewServiceContext(c config.Config) (*ServiceContext, error) {
	svc := &ServiceContext{Config: c}
	svc.SupabaseArticles = model.NewSupabaseArticleClient(c.SupabaseURL, c.SupabasePublishableKey)

	if c.DatabaseURL == "" {
		return svc, nil
	}

	db, err := sql.Open("pgx", c.DatabaseURL)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	svc.DB = db
	svc.HasDatabse = true
	svc.Users = model.NewUserModel(db)
	svc.Sessions = model.NewAuthSessionModel(db)
	svc.Inquiries = model.NewContactInquiryModel(db)
	svc.Articles = model.NewArticleModel(db)

	return svc, nil
}

func (s *ServiceContext) Close() {
	if s.DB != nil {
		_ = s.DB.Close()
	}
}
