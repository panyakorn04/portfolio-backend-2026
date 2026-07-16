package model

import (
	"encoding/json"
	"testing"
)

func TestConvertTranslationRowIncludesContent(t *testing.T) {
	t.Parallel()

	row := articleTranslationRow{
		ID:          "translation-1",
		Locale:      "en",
		Title:       "MDX article",
		Summary:     "Summary",
		Lead:        "Lead",
		ReadingTime: "4 min",
		Content:     "# Hello\n\nThis is **MDX** content.",
		Sections:    json.RawMessage(`[]`),
	}

	translation, err := convertTranslationRow(row)
	if err != nil {
		t.Fatalf("convertTranslationRow() error = %v", err)
	}
	if translation.Content != row.Content {
		t.Fatalf("Content = %q, want %q", translation.Content, row.Content)
	}
}

func TestTranslationBodiesPersistsContent(t *testing.T) {
	t.Parallel()

	const content = "# Hello\n\nPersist this content."
	bodies, err := translationBodies("article-1", []ArticleTranslation{
		{
			Locale:      "en",
			Title:       "Title",
			Summary:     "Summary",
			Lead:        "Lead",
			ReadingTime: "4 min",
			Content:     content,
			Sections:    []ArticleSection{},
		},
	})
	if err != nil {
		t.Fatalf("translationBodies() error = %v", err)
	}
	if len(bodies) != 1 {
		t.Fatalf("len(bodies) = %d, want 1", len(bodies))
	}
	if got := bodies[0]["content"]; got != content {
		t.Fatalf("content = %#v, want %q", got, content)
	}
}

func TestConvertSupabaseTranslationIncludesContent(t *testing.T) {
	t.Parallel()

	row := supabaseTranslationRow{
		ID:          "translation-1",
		Locale:      "th",
		Title:       "บทความ MDX",
		Summary:     "เรื่องย่อ",
		Lead:        "บทนำ",
		ReadingTime: "4 นาที",
		Content:     "# บทความ MDX\n\nเนื้อหา",
		Sections:    json.RawMessage(`[]`),
	}

	translation, err := convertSupabaseTranslation(row)
	if err != nil {
		t.Fatalf("convertSupabaseTranslation() error = %v", err)
	}
	if translation.Content != row.Content {
		t.Fatalf("Content = %q, want %q", translation.Content, row.Content)
	}
}

func TestPreserveMissingSections(t *testing.T) {
	t.Parallel()

	existing := []ArticleTranslation{
		{
			Locale: "en",
			Sections: []ArticleSection{
				{Heading: "Legacy", Paragraphs: []string{"Keep this paragraph."}},
			},
		},
	}
	incoming := []ArticleTranslation{
		{Locale: "en", Content: "# Updated content", Sections: []ArticleSection{}},
	}

	got := preserveMissingSections(existing, incoming)
	if len(got[0].Sections) != 1 || got[0].Sections[0].Heading != "Legacy" {
		t.Fatalf("Sections = %#v, want preserved legacy sections", got[0].Sections)
	}
}
