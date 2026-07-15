// Command createuser inserts or updates a staff user with a bcrypt password via Supabase REST.
//
// Usage:
//
//	NEXT_PUBLIC_SUPABASE_URL=... SUPABASE_SERVICE_ROLE_KEY=... go run ./cmd/createuser -email you@example.com -password secret -role editor -name "Your Name"
//
// Note: This tool only creates editor or viewer roles. Admin users must be
// created directly through the Supabase dashboard or by an existing admin.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/model"
)

var allowedRoles = []string{"editor", "viewer"}

func main() {
	email := flag.String("email", "", "user email")
	password := flag.String("password", "", "user password")
	role := flag.String("role", "editor", "staff role: editor|viewer")
	name := flag.String("name", "", "display name (optional)")
	flag.Parse()

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "email and password are required")
		os.Exit(1)
	}
	if !isAllowedRole(*role) {
		fmt.Fprintf(os.Stderr, "invalid role %q (use editor|viewer)\n", *role)
		os.Exit(1)
	}

	baseURL := os.Getenv("NEXT_PUBLIC_SUPABASE_URL")
	key := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
	if key == "" {
		key = os.Getenv("NEXT_PUBLIC_SUPABASE_PUBLISHABLE_KEY")
	}
	api := model.NewSupabaseREST(baseURL, key)
	if api == nil {
		fmt.Fprintln(os.Stderr, "NEXT_PUBLIC_SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY are required")
		os.Exit(1)
	}

	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		os.Exit(1)
	}

	var namePtr *string
	if strings.TrimSpace(*name) != "" {
		namePtr = name
	}

	userModel := model.NewUserModel(api)
	user, err := userModel.UpsertStaffUser(context.Background(), strings.ToLower(*email), namePtr, hash, *role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "upsert user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("user %s ready (id=%s, role=%s)\n", user.Email, user.ID, user.Role)
}

func isAllowedRole(role string) bool {
	for _, r := range allowedRoles {
		if r == role {
			return true
		}
	}
	return false
}
