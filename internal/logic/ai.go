package logic

import (
	"fmt"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"
)

type ContactSummary struct {
	Provider string `json:"provider"`
	Mode     string `json:"mode"`
	Summary  string `json:"summary"`
	Prompt   string `json:"prompt"`
}

func buildContactSummaryPrompt(inquiry *model.ContactInquiry) string {
	company := "-"
	if inquiry.Company != nil {
		company = *inquiry.Company
	}
	return fmt.Sprintf(
		"Summarize this contact inquiry for admin follow-up.\nName: %s\nEmail: %s\nCompany: %s\nSubject: %s\nLocale: %s\nMessage: %s",
		inquiry.Name, inquiry.Email, company, inquiry.Subject, inquiry.Locale, inquiry.Message)
}

// GenerateContactSummary mirrors the stub behavior of the original service.
func GenerateContactSummary(svcCtx *svc.ServiceContext, inquiry *model.ContactInquiry) ContactSummary {
	provider := svcCtx.Config.AiProvider
	if provider == "" {
		provider = "stub"
	}
	prompt := buildContactSummaryPrompt(inquiry)

	if svcCtx.Config.AiApiKey == "" {
		return ContactSummary{
			Provider: provider,
			Mode:     "stub",
			Summary:  fmt.Sprintf("Follow up with %s about %q. They mentioned: %s", inquiry.Name, inquiry.Subject, inquiry.Message),
			Prompt:   prompt,
		}
	}

	return ContactSummary{
		Provider: provider,
		Mode:     "stub",
		Summary:  fmt.Sprintf("AI provider wiring is reserved for %s. Prompt prepared successfully for inquiry %s.", provider, inquiry.ID),
		Prompt:   prompt,
	}
}
