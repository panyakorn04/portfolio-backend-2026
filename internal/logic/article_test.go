package logic

import "testing"

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
	// Only an "en" translation provided; "th" is required.
	payload := ArticlePayload{
		Category: "Engineering",
		Status:   "draft",
		Translations: []RawTranslation{
			{
				Locale: "en", Title: "Title", Summary: "Summary",
				Lead: "Lead", ReadingTime: "5 min",
				Sections: []struct {
					Heading    string   `json:"heading"`
					Paragraphs []string `json:"paragraphs"`
				}{{Heading: "H", Paragraphs: []string{"p"}}},
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
