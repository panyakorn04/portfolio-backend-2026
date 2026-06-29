package logic

import (
	"regexp"
	"strings"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/response"
)

var ArticleStatuses = []string{"draft", "published"}
var ArticleLocales = []string{"en", "th"}

var (
	slugInvalidChars = regexp.MustCompile(`[^a-z0-9\s-]`)
	slugSpaces       = regexp.MustCompile(`\s+`)
	slugDashes       = regexp.MustCompile(`-+`)
)

// BuildArticleSlug mirrors the original slugify behavior.
func BuildArticleSlug(value string) string {
	s := strings.ToLower(value)
	s = slugInvalidChars.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	s = slugSpaces.ReplaceAllString(s, "-")
	s = slugDashes.ReplaceAllString(s, "-")
	runes := []rune(s)
	if len(runes) > 80 {
		runes = runes[:80]
	}
	return string(runes)
}

func isStatusValid(status string) bool {
	for _, s := range ArticleStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// RawTranslation is the untrusted translation payload.
type RawTranslation struct {
	Locale      string `json:"locale"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Lead        string `json:"lead"`
	ReadingTime string `json:"readingTime"`
	Sections    []struct {
		Heading    string   `json:"heading"`
		Paragraphs []string `json:"paragraphs"`
	} `json:"sections"`
}

// ArticlePayload is the untrusted create/update body.
type ArticlePayload struct {
	Slug         string           `json:"slug"`
	Category     string           `json:"category"`
	Status       string           `json:"status"`
	Translations []RawTranslation `json:"translations"`
}

func validateSections(raw RawTranslation, locale string, details *[]response.ErrorDetail) []model.ArticleSection {
	if len(raw.Sections) == 0 {
		*details = append(*details, response.ErrorDetail{
			Field: "translations." + locale + ".sections", Message: "At least one section is required.",
		})
		return nil
	}

	var parsed []model.ArticleSection
	for i, section := range raw.Sections {
		idx := itoa(i)
		heading := strings.TrimSpace(section.Heading)
		if heading == "" {
			*details = append(*details, response.ErrorDetail{
				Field: "translations." + locale + ".sections." + idx + ".heading", Message: "Section heading is required.",
			})
		}

		validParagraphs := len(section.Paragraphs) > 0
		paragraphs := make([]string, 0, len(section.Paragraphs))
		for _, p := range section.Paragraphs {
			tp := strings.TrimSpace(p)
			if tp == "" {
				validParagraphs = false
				break
			}
			paragraphs = append(paragraphs, tp)
		}

		if !validParagraphs {
			*details = append(*details, response.ErrorDetail{
				Field:   "translations." + locale + ".sections." + idx + ".paragraphs",
				Message: "Each section needs at least one non-empty paragraph.",
			})
			continue
		}

		if heading != "" {
			parsed = append(parsed, model.ArticleSection{Heading: heading, Paragraphs: paragraphs})
		}
	}
	return parsed
}

// ValidateArticlePayload validates and normalizes the payload into model.ArticleInput.
func ValidateArticlePayload(payload ArticlePayload) (*model.ArticleInput, []response.ErrorDetail) {
	var details []response.ErrorDetail

	slugSource := strings.TrimSpace(payload.Slug)
	if slugSource == "" && len(payload.Translations) > 0 {
		slugSource = strings.TrimSpace(payload.Translations[0].Title)
	}
	slug := BuildArticleSlug(slugSource)
	if slug == "" {
		details = append(details, response.ErrorDetail{Field: "slug", Message: "Provide a slug or an English title to generate one."})
	}

	if strings.TrimSpace(payload.Category) == "" {
		details = append(details, response.ErrorDetail{Field: "category", Message: "Category is required."})
	}

	status := strings.TrimSpace(payload.Status)
	if status == "" {
		status = "draft"
	}
	if !isStatusValid(status) {
		details = append(details, response.ErrorDetail{Field: "status", Message: "Use one of: " + strings.Join(ArticleStatuses, ", ") + "."})
	}

	var translations []model.ArticleTranslation
	for _, locale := range ArticleLocales {
		var raw *RawTranslation
		for i := range payload.Translations {
			if payload.Translations[i].Locale == locale {
				raw = &payload.Translations[i]
				break
			}
		}
		if raw == nil {
			details = append(details, response.ErrorDetail{
				Field: "translations." + locale, Message: "Translation for \"" + locale + "\" is required.",
			})
			continue
		}

		title := strings.TrimSpace(raw.Title)
		summary := strings.TrimSpace(raw.Summary)
		lead := strings.TrimSpace(raw.Lead)
		readingTime := strings.TrimSpace(raw.ReadingTime)

		if title == "" {
			details = append(details, response.ErrorDetail{Field: "translations." + locale + ".title", Message: "Title is required."})
		}
		if summary == "" {
			details = append(details, response.ErrorDetail{Field: "translations." + locale + ".summary", Message: "Summary is required."})
		}
		if lead == "" {
			details = append(details, response.ErrorDetail{Field: "translations." + locale + ".lead", Message: "Lead is required."})
		}
		if readingTime == "" {
			details = append(details, response.ErrorDetail{Field: "translations." + locale + ".readingTime", Message: "Reading time is required."})
		}

		sections := validateSections(*raw, locale, &details)

		if title != "" && summary != "" && lead != "" && readingTime != "" && len(sections) > 0 {
			translations = append(translations, model.ArticleTranslation{
				Locale: locale, Title: title, Summary: summary,
				Lead: lead, ReadingTime: readingTime, Sections: sections,
			})
		}
	}

	if len(details) > 0 {
		return nil, details
	}

	return &model.ArticleInput{
		Slug: slug, Category: strings.TrimSpace(payload.Category),
		Status: status, Translations: translations,
	}, nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
