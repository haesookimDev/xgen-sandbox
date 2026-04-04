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

// Claims represents JWT-like claims for Phase 1 (simple HMAC-based tokens).
type Claims struct {
	Subject   string    `json:"sub"`
	ExpiresAt time.Time `json:"exp"`
	IssuedAt  time.Time `json:"iat"`
}

// Authenticator handles API key validation and token generation.
type Authenticator struct {
	apiKey    string
	jwtSecret []byte
}

// NewAuthenticator creates a new authenticator.
func NewAuthenticator(apiKey, jwtSecret string) *Authenticator {
	return &Authenticator{
		apiKey:    apiKey,
		jwtSecret: []byte(jwtSecret),
	}
}

// GenerateToken creates a signed token from an API key.
func (a *Authenticator) GenerateToken(apiKey string) (string, time.Time, error) {
	if apiKey != a.apiKey {
		return "", time.Time{}, fmt.Errorf("invalid API key")
	}

	expiresAt := time.Now().Add(15 * time.Minute)
	claims := Claims{
		Subject:   "default",
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
		auth := r.Header.Get("Authorization")
		if auth == "" {
			// Also check query param for WebSocket connections
			if token := r.URL.Query().Get("token"); token != "" {
				auth = "Bearer " + token
			}
		}

		if auth == "" {
			http.Error(w, `{"error":"missing authorization"}`, http.StatusUnauthorized)
			return
		}

		parts := strings.SplitN(auth, " ", 2)
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
			if parts[1] != a.apiKey {
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}
			claims := &Claims{Subject: "default"}
			ctx := context.WithValue(r.Context(), userContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))

		default:
			http.Error(w, `{"error":"unsupported auth scheme"}`, http.StatusUnauthorized)
		}
	})
}

// GetClaims extracts claims from the request context.
func GetClaims(ctx context.Context) *Claims {
	if v := ctx.Value(userContextKey); v != nil {
		return v.(*Claims)
	}
	return nil
}
