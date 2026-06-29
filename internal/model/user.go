package model

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type UserModel struct {
	db *sql.DB
}

func NewUserModel(db *sql.DB) *UserModel {
	return &UserModel{db: db}
}

func (m *UserModel) FindByEmail(ctx context.Context, email string) (*User, error) {
	const query = `SELECT "id", "email", "name", "passwordHash", "role", "createdAt", "updatedAt"
		FROM "User" WHERE "email" = $1`

	var u User
	err := m.db.QueryRowContext(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (m *UserModel) List(ctx context.Context) ([]PublicUser, error) {
	const query = `SELECT "id", "email", "name", "role" FROM "User" ORDER BY "createdAt" ASC`

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PublicUser, 0)
	for rows.Next() {
		var u PublicUser
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role); err != nil {
			return nil, err
		}
		items = append(items, u)
	}
	return items, rows.Err()
}

// UpdateRole updates a user's role and returns the public record.
// Returns ErrNotFound when no user matches the id.
func (m *UserModel) UpdateRole(ctx context.Context, id, role string) (*PublicUser, error) {
	const query = `UPDATE "User" SET "role" = $1, "updatedAt" = now()
		WHERE "id" = $2 RETURNING "id", "email", "name", "role"`

	var u PublicUser
	err := m.db.QueryRowContext(ctx, query, role, id).Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (m *UserModel) Create(ctx context.Context, email string, name *string, passwordHash, role string) (*PublicUser, error) {
	const query = `INSERT INTO "User" ("id", "email", "name", "passwordHash", "role", "updatedAt")
		VALUES ($1, $2, $3, $4, $5, now()) RETURNING "id", "email", "name", "role"`

	var u PublicUser
	err := m.db.QueryRowContext(ctx, query, newID(), email, name, passwordHash, role).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

type AuthSessionModel struct {
	db *sql.DB
}

func NewAuthSessionModel(db *sql.DB) *AuthSessionModel {
	return &AuthSessionModel{db: db}
}

func (m *AuthSessionModel) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (*AuthSession, error) {
	const query = `INSERT INTO "AuthSession" ("id", "userId", "sessionTokenHash", "expiresAt")
		VALUES ($1, $2, $3, $4) RETURNING "id", "expiresAt", "createdAt", "lastSeenAt"`

	var s AuthSession
	err := m.db.QueryRowContext(ctx, query, newID(), userID, tokenHash, expiresAt).
		Scan(&s.ID, &s.ExpiresAt, &s.CreatedAt, &s.LastSeenAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *AuthSessionModel) FindByTokenHash(ctx context.Context, tokenHash string) (*AuthSessionWithUser, error) {
	const query = `SELECT s."id", s."expiresAt", u."id", u."email", u."name", u."role"
		FROM "AuthSession" s
		JOIN "User" u ON u."id" = s."userId"
		WHERE s."sessionTokenHash" = $1`

	var s AuthSessionWithUser
	err := m.db.QueryRowContext(ctx, query, tokenHash).Scan(
		&s.ID, &s.ExpiresAt, &s.User.ID, &s.User.Email, &s.User.Name, &s.User.Role,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *AuthSessionModel) Touch(ctx context.Context, tokenHash string) error {
	const query = `UPDATE "AuthSession" SET "lastSeenAt" = now() WHERE "sessionTokenHash" = $1`
	_, err := m.db.ExecContext(ctx, query, tokenHash)
	return err
}

func (m *AuthSessionModel) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	const query = `DELETE FROM "AuthSession" WHERE "sessionTokenHash" = $1`
	_, err := m.db.ExecContext(ctx, query, tokenHash)
	return err
}

func (m *AuthSessionModel) ListForUser(ctx context.Context, userID string) ([]AuthSession, error) {
	const query = `SELECT "id", "expiresAt", "createdAt", "lastSeenAt"
		FROM "AuthSession" WHERE "userId" = $1 ORDER BY "createdAt" DESC`

	rows, err := m.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AuthSession, 0)
	for rows.Next() {
		var s AuthSession
		if err := rows.Scan(&s.ID, &s.ExpiresAt, &s.CreatedAt, &s.LastSeenAt); err != nil {
			return nil, err
		}
		items = append(items, s)
	}
	return items, rows.Err()
}

// DeleteByID removes a session scoped to a user. Returns the number of rows deleted.
func (m *AuthSessionModel) DeleteByID(ctx context.Context, sessionID, userID string) (int64, error) {
	const query = `DELETE FROM "AuthSession" WHERE "id" = $1 AND "userId" = $2`
	res, err := m.db.ExecContext(ctx, query, sessionID, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (m *AuthSessionModel) DeleteAllForUser(ctx context.Context, userID string) error {
	const query = `DELETE FROM "AuthSession" WHERE "userId" = $1`
	_, err := m.db.ExecContext(ctx, query, userID)
	return err
}
