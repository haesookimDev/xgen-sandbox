package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const userContextKey contextKey = "user"

// Role represents a user's access level.
type Role string

const (
	RoleAdmin  Role = "admin"
	RoleUser   Role = "user"
	RoleViewer Role = "viewer"
)

// Permission represents an action that can be authorized.
type Permission string

const (
	PermSandboxCreate Permission = "sandbox:create"
	PermSandboxRead   Permission = "sandbox:read"
	PermSandboxWrite  Permission = "sandbox:write"
	PermSandboxDelete Permission = "sandbox:delete"
	PermSandboxExec   Permission = "sandbox:exec"
	PermSandboxFiles  Permission = "sandbox:files"
)

// rolePermissions maps each role to its allowed permissions.
var rolePermissions = map[Role][]Permission{
	RoleAdmin: {
		PermSandboxCreate, PermSandboxRead, PermSandboxWrite,
		PermSandboxDelete, PermSandboxExec, PermSandboxFiles,
	},
	RoleUser: {
		PermSandboxCreate, PermSandboxRead, PermSandboxWrite,
		PermSandboxExec, PermSandboxFiles,
	},
	RoleViewer: {
		PermSandboxRead,
	},
}

// HasPermission checks whether a role has the given permission.
func (r Role) HasPermission(perm Permission) bool {
	for _, p := range rolePermissions[r] {
		if p == perm {
			return true
		}
	}
	return false
}

// Claims represents JWT-like claims with RBAC role.
type Claims struct {
	Subject   string    `json:"sub"`
	Role      Role      `json:"role"`
	ExpiresAt time.Time `json:"exp"`
	IssuedAt  time.Time `json:"iat"`
}

// APIKeyEntry represents a configured API key with its associated role.
type APIKeyEntry struct {
	Key  string
	Role Role
}

// Authenticator handles API key validation and token generation.
type Authenticator struct {
	apiKeys   map[string]Role // apiKey -> role
	jwtSecret []byte
}

// NewAuthenticator creates a new authenticator.
// The first key is the default admin key for backward compatibility.
func NewAuthenticator(apiKey, jwtSecret string) *Authenticator {
	return &Authenticator{
		apiKeys:   map[string]Role{apiKey: RoleAdmin},
		jwtSecret: []byte(jwtSecret),
	}
}

// NewAuthenticatorWithKeys creates an authenticator with multiple API keys.
func NewAuthenticatorWithKeys(keys []APIKeyEntry, jwtSecret string) *Authenticator {
	m := make(map[string]Role, len(keys))
	for _, k := range keys {
		m[k.Key] = k.Role
	}
	return &Authenticator{
		apiKeys:   m,
		jwtSecret: []byte(jwtSecret),
	}
}

// GenerateToken creates a signed token from an API key.
func (a *Authenticator) GenerateToken(apiKey string) (string, time.Time, error) {
	role, ok := a.apiKeys[apiKey]
	if !ok {
		return "", time.Time{}, fmt.Errorf("invalid API key")
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	claims := Claims{
		Subject:   "default",
		Role:      role,
		ExpiresAt: expiresAt,
		IssuedAt:  time.Now(),
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", time.Time{}, err
	}

	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	sig := a.sign(payload)
	token := payload + "." + sig

	return token, expiresAt, nil
}

// ValidateToken verifies a token and returns its claims.
func (a *Authenticator) ValidateToken(token string) (*Claims, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid token format")
	}

	expectedSig := a.sign(parts[0])
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode claims: %w", err)
	}

	var claims Claims
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	if time.Now().After(claims.ExpiresAt) {
		return nil, fmt.Errorf("token expired")
	}

	return &claims, nil
}

func (a *Authenticator) sign(data string) string {
	h := hmac.New(sha256.New, a.jwtSecret)
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// Middleware returns an HTTP middleware that validates tokens.
// It accepts both "Bearer <token>" and "ApiKey <key>" in the Authorization header.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			if token := r.URL.Query().Get("token"); token != "" {
				authHeader = "Bearer " + token
			}
		}

		if authHeader == "" {
			http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 {
			http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
			return
		}

		switch parts[0] {
		case "Bearer":
			claims, err := a.ValidateToken(parts[1])
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), userContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))

		case "ApiKey":
			role, ok := a.apiKeys[parts[1]]
			if !ok {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
			claims := &Claims{Subject: "default", Role: role}
			ctx := context.WithValue(r.Context(), userContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))

		default:
			http.Error(w, `{"error":"unsupported auth scheme"}`, http.StatusUnauthorized)
		}
	})
}

// RequirePermission returns middleware that checks a specific permission.
func RequirePermission(perm Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetClaims(r.Context())
			if claims == nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			if !claims.Role.HasPermission(perm) {
				http.Error(w, `{"error":"forbidden","code":"insufficient_permissions"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r.WithContext(r.Context()))
		})
	}
}

// GetClaims extracts claims from the request context.
func GetClaims(ctx context.Context) *Claims {
	if v := ctx.Value(userContextKey); v != nil {
		return v.(*Claims)
	}
	return nil
}
