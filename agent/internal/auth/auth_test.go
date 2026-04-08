package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		role Role
		perm Permission
		want bool
	}{
		{RoleAdmin, PermSandboxCreate, true},
		{RoleAdmin, PermSandboxDelete, true},
		{RoleAdmin, PermSandboxRead, true},
		{RoleAdmin, PermSandboxExec, true},
		{RoleUser, PermSandboxCreate, true},
		{RoleUser, PermSandboxRead, true},
		{RoleUser, PermSandboxDelete, false},
		{RoleUser, PermSandboxExec, true},
		{RoleViewer, PermSandboxRead, true},
		{RoleViewer, PermSandboxCreate, false},
		{RoleViewer, PermSandboxDelete, false},
		{RoleViewer, PermSandboxExec, false},
		{Role("unknown"), PermSandboxRead, false},
	}

	for _, tt := range tests {
		got := tt.role.HasPermission(tt.perm)
		if got != tt.want {
			t.Errorf("Role(%q).HasPermission(%q) = %v, want %v", tt.role, tt.perm, got, tt.want)
		}
	}
}

func TestGenerateToken_ValidAPIKey(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	token, expiresAt, err := auth.GenerateToken("test-key")
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if token == "" {
		t.Error("expected non-empty token")
	}
	if expiresAt.IsZero() {
		t.Error("expected non-zero expiration")
	}
}

func TestGenerateToken_InvalidAPIKey(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	_, _, err := auth.GenerateToken("wrong-key")
	if err == nil {
		t.Fatal("expected error for invalid API key")
	}
}

func TestValidateToken_Roundtrip(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	token, _, err := auth.GenerateToken("test-key")
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}

	claims, err := auth.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error: %v", err)
	}
	if claims.Subject != "default" {
		t.Errorf("subject: expected default, got %q", claims.Subject)
	}
	if claims.Role != RoleAdmin {
		t.Errorf("role: expected admin, got %q", claims.Role)
	}
}

func TestValidateToken_InvalidFormat(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	_, err := auth.ValidateToken("no-dot-separator")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestValidateToken_InvalidSignature(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	token, _, _ := auth.GenerateToken("test-key")

	// Corrupt the signature by flipping a character
	corrupted := token[:len(token)-1] + "X"
	_, err := auth.ValidateToken(corrupted)
	if err == nil {
		t.Fatal("expected error for invalid signature")
	}
}

func TestNewAuthenticatorWithKeys(t *testing.T) {
	keys := []APIKeyEntry{
		{Key: "admin-key", Role: RoleAdmin},
		{Key: "user-key", Role: RoleUser},
		{Key: "viewer-key", Role: RoleViewer},
	}
	auth := NewAuthenticatorWithKeys(keys, "secret")

	// Admin key should generate admin token
	token, _, err := auth.GenerateToken("admin-key")
	if err != nil {
		t.Fatalf("GenerateToken(admin) error: %v", err)
	}
	claims, _ := auth.ValidateToken(token)
	if claims.Role != RoleAdmin {
		t.Errorf("expected admin role, got %q", claims.Role)
	}

	// User key should generate user token
	token, _, err = auth.GenerateToken("user-key")
	if err != nil {
		t.Fatalf("GenerateToken(user) error: %v", err)
	}
	claims, _ = auth.ValidateToken(token)
	if claims.Role != RoleUser {
		t.Errorf("expected user role, got %q", claims.Role)
	}
}

func TestMiddleware_BearerToken(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")
	token, _, _ := auth.GenerateToken("test-key")

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims := GetClaims(r.Context())
		if claims == nil {
			t.Error("expected claims in context")
		}
		if claims.Role != RoleAdmin {
			t.Errorf("expected admin role, got %q", claims.Role)
		}
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
}

func TestMiddleware_ApiKey(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		claims := GetClaims(r.Context())
		if claims == nil {
			t.Error("expected claims in context")
		}
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "ApiKey test-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_UnsupportedScheme(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")

	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_QueryToken(t *testing.T) {
	auth := NewAuthenticator("test-key", "test-secret")
	token, _, _ := auth.GenerateToken("test-key")

	called := false
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/test?token="+token, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestRequirePermission_Allowed(t *testing.T) {
	claims := &Claims{Subject: "test", Role: RoleAdmin}
	ctx := context.WithValue(context.Background(), userContextKey, claims)

	called := false
	handler := RequirePermission(PermSandboxDelete)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("DELETE", "/test", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestRequirePermission_Forbidden(t *testing.T) {
	claims := &Claims{Subject: "test", Role: RoleViewer}
	ctx := context.WithValue(context.Background(), userContextKey, claims)

	handler := RequirePermission(PermSandboxCreate)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("POST", "/test", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status: expected 403, got %d", rec.Code)
	}
}

func TestRequirePermission_NoClaims(t *testing.T) {
	handler := RequirePermission(PermSandboxRead)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: expected 401, got %d", rec.Code)
	}
}

func TestGetClaims_NilContext(t *testing.T) {
	claims := GetClaims(context.Background())
	if claims != nil {
		t.Error("expected nil claims for empty context")
	}
}
