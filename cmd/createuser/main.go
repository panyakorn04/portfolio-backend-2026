// Command createuser inserts or updates a staff user with a bcrypt password.
//
// Usage:
//
//	DATABASE_URL=... go run ./cmd/createuser -email you@example.com -password secret -role admin -name "Your Name"
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	"portfolio-backend/internal/auth"
	"portfolio-backend/internal/model"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	email := flag.String("email", "", "user email")
	password := flag.String("password", "", "user password")
	role := flag.String("role", "admin", "staff role: admin|editor|viewer")
	name := flag.String("name", "", "display name (optional)")
	flag.Parse()

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "email and password are required")
		os.Exit(1)
	}
	if !auth.IsStaffRole(*role) {
		fmt.Fprintf(os.Stderr, "invalid role %q (use admin|editor|viewer)\n", *role)
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(1)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	hash, err := auth.HashPassword(*password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash password: %v\n", err)
		os.Exit(1)
	}

	var namePtr *string
	if strings.TrimSpace(*name) != "" {
		namePtr = name
	}

	const query = `INSERT INTO "User" ("id", "email", "name", "passwordHash", "role", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT ("email") DO UPDATE
		SET "name" = EXCLUDED."name", "passwordHash" = EXCLUDED."passwordHash",
		    "role" = EXCLUDED."role", "updatedAt" = now()
		RETURNING "id"`

	var id string
	err = db.QueryRowContext(context.Background(), query,
		model.GenerateID(), strings.ToLower(*email), namePtr, hash, *role).Scan(&id)
	if err != nil {
		fmt.Fprintf(os.Stderr, "upsert user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("user %s ready (id=%s, role=%s)\n", *email, id, *role)
}
