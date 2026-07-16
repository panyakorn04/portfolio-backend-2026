package model

import (
	"errors"
	"time"
)

var ErrNotFound = errors.New("record not found")

type User struct {
	ID           string
	Email        string
	Name         *string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type PublicUser struct {
	ID    string  `json:"id"`
	Email string  `json:"email"`
	Name  *string `json:"name"`
	Role  string  `json:"role"`
}

type AuthSession struct {
	ID         string    `json:"id"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
}

type AuthSessionWithUser struct {
	ID        string
	ExpiresAt time.Time
	User      PublicUser
}

type PortfolioChatSession struct {
	ID            string    `json:"id"`
	VisitorIDHash string    `json:"-"`
	ThreadID      string    `json:"threadId"`
	Locale        string    `json:"locale"`
	Title         *string   `json:"title"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	LastSeenAt    time.Time `json:"lastSeenAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type PortfolioChatMessage struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId"`
	Role      string         `json:"role"`
	Type      string         `json:"type"`
	Content   string         `json:"content"`
	CreatedAt time.Time      `json:"createdAt"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ContactInquiry struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Email        string     `json:"email"`
	Company      *string    `json:"company"`
	Subject      string     `json:"subject"`
	Message      string     `json:"message"`
	Locale       string     `json:"locale"`
	DeliveryMode string     `json:"deliveryMode"`
	Status       string     `json:"status"`
	InternalNote *string    `json:"internalNote"`
	HandledAt    *time.Time `json:"handledAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

type ContactInquiryActivity struct {
	ID               string    `json:"id"`
	ActorType        string    `json:"actorType"`
	ActorLabel       string    `json:"actorLabel"`
	EventType        string    `json:"eventType"`
	StatusFrom       *string   `json:"statusFrom"`
	StatusTo         *string   `json:"statusTo"`
	InternalNoteFrom *string   `json:"internalNoteFrom"`
	InternalNoteTo   *string   `json:"internalNoteTo"`
	CreatedAt        time.Time `json:"createdAt"`
}

type ContactInquiryDetail struct {
	ContactInquiry
	Activities []ContactInquiryActivity `json:"activities"`
}

type ArticleSection struct {
	Heading    string   `json:"heading"`
	Paragraphs []string `json:"paragraphs"`
}

type ArticleTranslation struct {
	ID          string `json:"id"`
	Locale      string `json:"locale"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Lead        string `json:"lead"`
	ReadingTime string `json:"readingTime"`
	Content     string `json:"content"`
	// Sections is retained for legacy reads; Content is the authoritative article body.
	Sections []ArticleSection `json:"sections"`
}

type Article struct {
	ID           string               `json:"id"`
	Slug         string               `json:"slug"`
	Category     string               `json:"category"`
	Status       string               `json:"status"`
	PublishedAt  *time.Time           `json:"publishedAt"`
	CreatedAt    time.Time            `json:"createdAt"`
	UpdatedAt    time.Time            `json:"updatedAt"`
	Translations []ArticleTranslation `json:"translations"`
}

type ArticleInput struct {
	Slug         string
	Category     string
	Status       string
	PublishedAt  *time.Time
	Translations []ArticleTranslation
}
