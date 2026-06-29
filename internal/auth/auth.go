package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"portfolio-backend/internal/model"
	"portfolio-backend/internal/svc"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

const (
	SessionCookieName    = "portfolio_admin_session"
	SessionMaxAgeSeconds = 60 * 60 * 24 * 7
)

var StaffRoles = []string{"admin", "editor", "viewer"}

func IsStaffRole(role string) bool {
	for _, r := range StaffRoles {
		if r == role {
			return true
		}
	}
	return false
}

// Legacy scrypt parameters, matching Node's crypto.scryptSync defaults used by
// the original Next.js backend: N=16384, r=8, p=1, keyLen=64.
const (
	legacyScryptN      = 16384
	legacyScryptR      = 8
	legacyScryptP      = 1
	legacyScryptKeyLen = 64
)

// HashPassword returns a bcrypt hash of the password. New users use bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// VerifyPassword reports whether the password matches the stored hash.
//
// It supports two formats:
//   - bcrypt (new users created by this service): starts with "$2".
//   - legacy scrypt "salt:key" (hex) from the original Next.js backend, so
//     existing users keep working without a re-hash.
func VerifyPassword(password, hash string) bool {
	if strings.HasPrefix(hash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
	}
	return verifyLegacyScrypt(password, hash)
}

func verifyLegacyScrypt(password, hash string) bool {
	parts := strings.SplitN(hash, ":", 2)
	if len(parts) != 2 {
		return false
	}

	salt, err := hex.DecodeString(parts[0])
	if err != nil {
		return false
	}
	stored, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}

	derived, err := scrypt.Key([]byte(password), salt,
		legacyScryptN, legacyScryptR, legacyScryptP, legacyScryptKeyLen)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare(derived, stored) == 1
}

// CreateRawSessionToken generates a random 32-byte base64url token.
func CreateRawSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashSessionToken returns the hex sha256 of the raw token (stored in the DB).
func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func SessionExpiry() time.Time {
	return time.Now().Add(SessionMaxAgeSeconds * time.Second)
}

func GetCookieValue(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func getBearerToken(r *http.Request) string {
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(authz, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(authz, prefix))
}

// AccessVia describes how the caller authenticated.
type AccessVia string

const (
	ViaBearer  AccessVia = "bearer"
	ViaSession AccessVia = "session"
)

type AccessContext struct {
	Kind string // "admin" | "internal"
	Via  AccessVia
	User *model.PublicUser
}

// AccessError carries an HTTP status with a message.
type AccessError struct {
	Status  int
	Message string
}

func (e *AccessError) Error() string { return e.Message }

func unauthorized(msg string) *AccessError {
	return &AccessError{Status: http.StatusUnauthorized, Message: msg}
}
func forbidden(msg string) *AccessError {
	return &AccessError{Status: http.StatusForbidden, Message: msg}
}

// RequireAdmin authenticates admin access via bearer token or session cookie.
func RequireAdmin(ctx context.Context, svcCtx *svc.ServiceContext, r *http.Request) (*AccessContext, *AccessError) {
	token := getBearerToken(r)
	sessionToken := GetCookieValue(r, SessionCookieName)

	if token != "" {
		if svcCtx.Config.AdminApiToken == "" {
			return nil, forbidden("ADMIN_API_TOKEN is not configured.")
		}
		if token != svcCtx.Config.AdminApiToken {
			return nil, forbidden("Admin token is invalid.")
		}
		return &AccessContext{Kind: "admin", Via: ViaBearer}, nil
	}

	if sessionToken != "" && svcCtx.HasDatabse {
		session, err := svcCtx.Sessions.FindByTokenHash(ctx, HashSessionToken(sessionToken))
		if err == nil && session != nil &&
			session.ExpiresAt.After(time.Now()) && IsStaffRole(session.User.Role) {
			_ = svcCtx.Sessions.Touch(ctx, HashSessionToken(sessionToken))
			user := session.User
			return &AccessContext{Kind: "admin", Via: ViaSession, User: &user}, nil
		}
	}

	if svcCtx.Config.AdminApiToken == "" {
		return nil, forbidden("Admin access is not configured. Create an admin user or set ADMIN_API_TOKEN.")
	}
	return nil, unauthorized("Admin access requires a valid session or bearer token.")
}

// RequireInternal authenticates internal access via bearer token only.
func RequireInternal(svcCtx *svc.ServiceContext, r *http.Request) (*AccessContext, *AccessError) {
	token := getBearerToken(r)
	if token == "" {
		return nil, unauthorized("Internal access requires a bearer token.")
	}
	if svcCtx.Config.InternalApiToken == "" {
		return nil, forbidden("INTERNAL_API_TOKEN is not configured.")
	}
	if token != svcCtx.Config.InternalApiToken {
		return nil, forbidden("Internal token is invalid.")
	}
	return &AccessContext{Kind: "internal", Via: ViaBearer}, nil
}

// AssertStaffRole enforces RBAC for session-based access. Bearer access bypasses role checks.
func AssertStaffRole(access *AccessContext, allowed []string) *AccessError {
	if access.Via == ViaBearer {
		return nil
	}
	if access.User == nil {
		return forbidden("This action requires a user-backed session.")
	}
	for _, role := range allowed {
		if access.User.Role == role {
			return nil
		}
	}
	return forbidden("This action requires one of these roles: " + strings.Join(allowed, ", ") + ".")
}
