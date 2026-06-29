package logic

import (
	"context"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
	"portfolio-backend/internal/svc"
)

type ContactSubmission struct {
	Name    string
	Email   string
	Company string
	Subject string
	Message string
	Locale  string
}

// ValidateContactSubmission normalizes input and returns validation details.
func ValidateContactSubmission(in ContactSubmission) (ContactSubmission, []response.ErrorDetail) {
	var details []response.ErrorDetail

	name := strings.TrimSpace(in.Name)
	email := strings.ToLower(strings.TrimSpace(in.Email))
	company := strings.TrimSpace(in.Company)
	subject := strings.TrimSpace(in.Subject)
	message := strings.TrimSpace(in.Message)
	locale := strings.ToLower(strings.TrimSpace(in.Locale))
	if locale == "" {
		locale = "en"
	}

	if len([]rune(name)) < 2 {
		details = append(details, response.ErrorDetail{Field: "name", Message: "Name must be at least 2 characters long."})
	}
	if !strings.Contains(email, "@") || strings.HasPrefix(email, "@") || strings.HasSuffix(email, "@") {
		details = append(details, response.ErrorDetail{Field: "email", Message: "Email must be a valid email address."})
	}
	if len([]rune(subject)) < 3 {
		details = append(details, response.ErrorDetail{Field: "subject", Message: "Subject must be at least 3 characters long."})
	}
	if len([]rune(message)) < 20 {
		details = append(details, response.ErrorDetail{Field: "message", Message: "Message must be at least 20 characters long."})
	}

	return ContactSubmission{
		Name: name, Email: email, Company: company,
		Subject: subject, Message: message, Locale: locale,
	}, details
}

type ContactResult struct {
	InquiryID    string `json:"inquiryId"`
	DeliveryMode string `json:"deliveryMode"`
	SubmittedAt  string `json:"submittedAt"`
}

// PersistAndDeliver stores the inquiry and best-effort delivers a webhook.
func PersistAndDeliver(ctx context.Context, svcCtx *svc.ServiceContext, sub ContactSubmission) (*ContactResult, error) {
	submittedAt := time.Now().UTC().Format(time.RFC3339)

	var company *string
	if sub.Company != "" {
		company = &sub.Company
	}

	inquiry, err := svcCtx.Inquiries.Create(ctx, model.ContactInquiry{
		Name: sub.Name, Email: sub.Email, Company: company,
		Subject: sub.Subject, Message: sub.Message, Locale: sub.Locale,
	})
	if err != nil {
		return nil, err
	}

	deliveryMode := "database"
	delivered := SendContactWebhook(svcCtx, map[string]any{
		"name": sub.Name, "email": sub.Email, "company": sub.Company,
		"subject": sub.Subject, "message": sub.Message, "locale": sub.Locale,
		"submittedAt": submittedAt,
	})
	if delivered {
		deliveryMode = "database+webhook"
	}

	if deliveryMode != inquiry.DeliveryMode {
		_ = svcCtx.Inquiries.UpdateDeliveryMode(ctx, inquiry.ID, deliveryMode)
	}

	return &ContactResult{
		InquiryID: inquiry.ID, DeliveryMode: deliveryMode, SubmittedAt: submittedAt,
	}, nil
}
