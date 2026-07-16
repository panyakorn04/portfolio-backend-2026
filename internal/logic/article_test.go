package logic

import (
	"strings"
	"testing"

	"portfolio-backend/internal/response"
)

func TestBuildArticleSlug(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Hello World", "hello-world"},
		{"  Trim  Spaces  ", "trim-spaces"},
		{"Special!@#Chars", "specialchars"},
		{"multiple---dashes", "multiple-dashes"},
		{"UPPER case", "upper-case"},
	}

	for _, tt := range tests {
		if got := BuildArticleSlug(tt.in); got != tt.want {
			t.Errorf("BuildArticleSlug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestValidateArticlePayloadRequiresBothLocales(t *testing.T) {
	payload := ArticlePayload{
		Category: "Engineering",
		Status:   "draft",
		Translations: []RawTranslation{
			{
				Locale: "en", Title: "Title", Summary: "Summary",
				Lead: "Lead", ReadingTime: "5 min", Content: "# Article",
			},
		},
	}

	input, details := ValidateArticlePayload(payload)
	if input != nil {
		t.Fatal("expected validation to fail when th translation is missing")
	}
	if len(details) == 0 {
		t.Fatal("expected validation details")
	}
}

func TestValidateArticlePayloadAcceptsMDXContent(t *testing.T) {
	t.Parallel()

	payload := ArticlePayload{
		Category: "Engineering",
		Status:   "draft",
		Translations: []RawTranslation{
			{
				Locale: "en", Title: "English title", Summary: "Summary",
				Lead: "Lead", ReadingTime: "5 min", Content: "# English\n\nArticle body.",
			},
			{
				Locale: "th", Title: "Thai title", Summary: "Summary",
				Lead: "Lead", ReadingTime: "5 min", Content: "# ไทย\n\nเนื้อหาบทความ",
			},
		},
	}

	input, details := ValidateArticlePayload(payload)
	if len(details) != 0 {
		t.Fatalf("ValidateArticlePayload() details = %#v, want none", details)
	}
	if input == nil {
		t.Fatal("ValidateArticlePayload() input = nil, want normalized input")
	}
	if got := input.Translations[0].Content; got != payload.Translations[0].Content {
		t.Fatalf("English content = %q, want %q", got, payload.Translations[0].Content)
	}
}

func TestValidateArticlePayloadRequiresContent(t *testing.T) {
	t.Parallel()

	payload := ArticlePayload{
		Category: "Engineering",
		Status:   "draft",
		Translations: []RawTranslation{
			{Locale: "en", Title: "English", Summary: "Summary", Lead: "Lead", ReadingTime: "5 min"},
			{Locale: "th", Title: "Thai", Summary: "Summary", Lead: "Lead", ReadingTime: "5 min"},
		},
	}

	input, details := ValidateArticlePayload(payload)
	if input != nil {
		t.Fatal("ValidateArticlePayload() input is non-nil, want validation failure")
	}
	assertValidationField(t, details, "translations.en.content")
	assertValidationField(t, details, "translations.th.content")
}

func TestValidateArticlePayloadRejectsLegacySectionsWithoutContent(t *testing.T) {
	t.Parallel()

	section := []struct {
		Heading    string   `json:"heading"`
		Paragraphs []string `json:"paragraphs"`
	}{{Heading: "Legacy", Paragraphs: []string{"Legacy paragraph."}}}
	payload := ArticlePayload{
		Category: "Engineering",
		Status:   "draft",
		Translations: []RawTranslation{
			{Locale: "en", Title: "English", Summary: "Summary", Lead: "Lead", ReadingTime: "5 min", Sections: section},
			{Locale: "th", Title: "Thai", Summary: "Summary", Lead: "Lead", ReadingTime: "5 min", Sections: section},
		},
	}

	input, details := ValidateArticlePayload(payload)
	if input != nil {
		t.Fatal("ValidateArticlePayload() input is non-nil, want content validation failure")
	}
	assertValidationField(t, details, "translations.en.content")
	assertValidationField(t, details, "translations.th.content")
}

func TestValidateArticlePayloadRejectsUnsafeOrOversizedContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "raw HTML", content: "# Article\n\n<script>alert('xss')</script>"},
		{name: "dangerous URL", content: "# Article\n\n[click](javascript:alert('xss'))"},
		{name: "oversized", content: strings.Repeat("a", maxArticleContentBytes+1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			payload := ArticlePayload{
				Category: "Engineering",
				Status:   "draft",
				Translations: []RawTranslation{
					{Locale: "en", Title: "English", Summary: "Summary", Lead: "Lead", ReadingTime: "5 min", Content: tt.content},
					{Locale: "th", Title: "Thai", Summary: "Summary", Lead: "Lead", ReadingTime: "5 min", Content: tt.content},
				},
			}

			input, details := ValidateArticlePayload(payload)
			if input != nil {
				t.Fatal("ValidateArticlePayload() input is non-nil, want content validation failure")
			}
			assertValidationField(t, details, "translations.en.content")
			assertValidationField(t, details, "translations.th.content")
		})
	}
}

func assertValidationField(t *testing.T, details []response.ErrorDetail, field string) {
	t.Helper()
	for _, detail := range details {
		if detail.Field == field {
			return
		}
	}
	t.Errorf("missing validation detail for %s", field)
}
