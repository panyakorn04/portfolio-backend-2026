package logic

import "testing"

func TestValidateContactSubmission(t *testing.T) {
	tests := []struct {
		name        string
		in          ContactSubmission
		wantDetails int
	}{
		{
			name: "valid",
			in: ContactSubmission{
				Name: "Alice", Email: "alice@example.com",
				Subject: "Hello", Message: "This is a sufficiently long message body.",
			},
			wantDetails: 0,
		},
		{
			name:        "all invalid",
			in:          ContactSubmission{Name: "a", Email: "bad", Subject: "x", Message: "short"},
			wantDetails: 4,
		},
		{
			name: "email missing at sign",
			in: ContactSubmission{
				Name: "Alice", Email: "aliceexample.com",
				Subject: "Hello", Message: "This is a sufficiently long message body.",
			},
			wantDetails: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, details := ValidateContactSubmission(tt.in)
			if len(details) != tt.wantDetails {
				t.Fatalf("got %d details, want %d: %+v", len(details), tt.wantDetails, details)
			}
		})
	}
}

func TestValidateContactSubmissionNormalizes(t *testing.T) {
	out, _ := ValidateContactSubmission(ContactSubmission{
		Name: "  Alice  ", Email: "  Alice@Example.com ",
		Subject: " Hello ", Message: "This is a sufficiently long message body.",
		Locale: "",
	})

	if out.Email != "alice@example.com" {
		t.Errorf("email not normalized: %q", out.Email)
	}
	if out.Name != "Alice" {
		t.Errorf("name not trimmed: %q", out.Name)
	}
	if out.Locale != "en" {
		t.Errorf("locale default not applied: %q", out.Locale)
	}
}
