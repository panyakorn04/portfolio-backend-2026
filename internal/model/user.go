package model

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type UserModel struct {
	api *SupabaseREST
}

func NewUserModel(api *SupabaseREST) *UserModel {
	return &UserModel{api: api}
}

type userRow struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Name         *string `json:"name"`
	PasswordHash string  `json:"passwordHash"`
	Role         string  `json:"role"`
	CreatedAt    string  `json:"createdAt"`
	UpdatedAt    string  `json:"updatedAt"`
}

func rowToUser(row userRow) User {
	return User{ID: row.ID, Email: row.Email, Name: row.Name, PasswordHash: row.PasswordHash, Role: row.Role,
		CreatedAt: timeFromString(row.CreatedAt), UpdatedAt: timeFromString(row.UpdatedAt)}
}

func rowToPublicUser(row userRow) PublicUser {
	return PublicUser{ID: row.ID, Email: row.Email, Name: row.Name, Role: row.Role}
}

func (m *UserModel) FindByEmail(ctx context.Context, email string) (*User, error) {
	values := url.Values{}
	values.Set("select", "id,email,name,passwordHash,role,createdAt,updatedAt")
	values.Set("email", "eq."+email)
	values.Set("limit", "1")
	var rows []userRow
	if _, err := m.api.request(ctx, http.MethodGet, "User", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	u := rowToUser(rows[0])
	return &u, nil
}

func (m *UserModel) List(ctx context.Context) ([]PublicUser, error) {
	values := url.Values{}
	values.Set("select", "id,email,name,role,createdAt")
	values.Set("order", "createdAt.asc")
	var rows []userRow
	if _, err := m.api.request(ctx, http.MethodGet, "User", values, nil, "", &rows); err != nil {
		return nil, err
	}
	items := make([]PublicUser, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToPublicUser(row))
	}
	return items, nil
}

// UpdateRole updates a user's role and returns the public record.
// Returns ErrNotFound when no user matches the id.
func (m *UserModel) UpdateRole(ctx context.Context, id, role string) (*PublicUser, error) {
	values := url.Values{}
	values.Set("id", "eq."+id)
	values.Set("select", "id,email,name,role")
	body := map[string]any{"role": role, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	var rows []userRow
	if _, err := m.api.request(ctx, http.MethodPatch, "User", values, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	u := rowToPublicUser(rows[0])
	return &u, nil
}

func (m *UserModel) Create(ctx context.Context, email string, name *string, passwordHash, role string) (*PublicUser, error) {
	body := map[string]any{"id": newID(), "email": strings.ToLower(email), "name": name, "passwordHash": passwordHash, "role": role, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	values := url.Values{}
	values.Set("select", "id,email,name,role")
	var rows []userRow
	if _, err := m.api.request(ctx, http.MethodPost, "User", values, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	u := rowToPublicUser(rows[0])
	return &u, nil
}

func (m *UserModel) UpsertStaffUser(ctx context.Context, email string, name *string, passwordHash, role string) (*PublicUser, error) {
	body := map[string]any{"id": newID(), "email": strings.ToLower(email), "name": name, "passwordHash": passwordHash, "role": role, "updatedAt": time.Now().UTC().Format(time.RFC3339)}
	values := url.Values{}
	values.Set("on_conflict", "email")
	values.Set("select", "id,email,name,role")
	var rows []userRow
	if _, err := m.api.request(ctx, http.MethodPost, "User", values, body, "resolution=merge-duplicates,return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	u := rowToPublicUser(rows[0])
	return &u, nil
}

type AuthSessionModel struct {
	api *SupabaseREST
}

func NewAuthSessionModel(api *SupabaseREST) *AuthSessionModel { return &AuthSessionModel{api: api} }

type authSessionRow struct {
	ID               string `json:"id"`
	UserID           string `json:"userId"`
	SessionTokenHash string `json:"sessionTokenHash"`
	ExpiresAt        string `json:"expiresAt"`
	CreatedAt        string `json:"createdAt"`
	LastSeenAt       string `json:"lastSeenAt"`
}

func rowToSession(row authSessionRow) AuthSession {
	return AuthSession{ID: row.ID, ExpiresAt: timeFromString(row.ExpiresAt), CreatedAt: timeFromString(row.CreatedAt), LastSeenAt: timeFromString(row.LastSeenAt)}
}

func (m *AuthSessionModel) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (*AuthSession, error) {
	body := map[string]any{"id": newID(), "userId": userID, "sessionTokenHash": tokenHash, "expiresAt": expiresAt.UTC().Format(time.RFC3339)}
	var rows []authSessionRow
	if _, err := m.api.request(ctx, http.MethodPost, "AuthSession", url.Values{}, body, "return=representation", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotFound
	}
	s := rowToSession(rows[0])
	return &s, nil
}

func (m *AuthSessionModel) FindByTokenHash(ctx context.Context, tokenHash string) (*AuthSessionWithUser, error) {
	values := url.Values{}
	values.Set("select", "id,userId,expiresAt")
	values.Set("sessionTokenHash", "eq."+tokenHash)
	values.Set("limit", "1")
	var rows []authSessionRow
	if _, err := m.api.request(ctx, http.MethodGet, "AuthSession", values, nil, "", &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	uValues := url.Values{}
	uValues.Set("select", "id,email,name,role")
	uValues.Set("id", "eq."+rows[0].UserID)
	uValues.Set("limit", "1")
	var users []userRow
	if _, err := m.api.request(ctx, http.MethodGet, "User", uValues, nil, "", &users); err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}
	return &AuthSessionWithUser{ID: rows[0].ID, ExpiresAt: timeFromString(rows[0].ExpiresAt), User: rowToPublicUser(users[0])}, nil
}

func (m *AuthSessionModel) Touch(ctx context.Context, tokenHash string) error {
	values := url.Values{}
	values.Set("sessionTokenHash", "eq."+tokenHash)
	_, err := m.api.request(ctx, http.MethodPatch, "AuthSession", values, map[string]any{"lastSeenAt": time.Now().UTC().Format(time.RFC3339)}, "", nil)
	return err
}

func (m *AuthSessionModel) DeleteByTokenHash(ctx context.Context, tokenHash string) error {
	values := url.Values{}
	values.Set("sessionTokenHash", "eq."+tokenHash)
	_, err := m.api.request(ctx, http.MethodDelete, "AuthSession", values, nil, "", nil)
	return err
}

func (m *AuthSessionModel) ListForUser(ctx context.Context, userID string) ([]AuthSession, error) {
	values := url.Values{}
	values.Set("select", "id,expiresAt,createdAt,lastSeenAt")
	values.Set("userId", "eq."+userID)
	values.Set("order", "createdAt.desc")
	var rows []authSessionRow
	if _, err := m.api.request(ctx, http.MethodGet, "AuthSession", values, nil, "", &rows); err != nil {
		return nil, err
	}
	items := make([]AuthSession, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToSession(row))
	}
	return items, nil
}

func (m *AuthSessionModel) DeleteByID(ctx context.Context, sessionID, userID string) (int64, error) {
	values := url.Values{}
	values.Set("id", "eq."+sessionID)
	values.Set("userId", "eq."+userID)
	var rows []authSessionRow
	_, err := m.api.request(ctx, http.MethodDelete, "AuthSession", values, nil, "return=representation", &rows)
	return int64(len(rows)), err
}

func (m *AuthSessionModel) DeleteAllForUser(ctx context.Context, userID string) error {
	values := url.Values{}
	values.Set("userId", "eq."+userID)
	_, err := m.api.request(ctx, http.MethodDelete, "AuthSession", values, nil, "", nil)
	return err
}
