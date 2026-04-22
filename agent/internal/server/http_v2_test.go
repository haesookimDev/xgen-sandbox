package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v2 "github.com/xgen-sandbox/agent/api/v2"
)

func TestV2_AuthToken_ValidKey(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	body, _ := json.Marshal(v2.AuthTokenRequest{APIKey: "test-key"})
	req := httptest.NewRequest("POST", "/api/v2/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp v2.AuthTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.ExpiresAtMs <= 0 {
		t.Errorf("expected positive ExpiresAtMs, got %d", resp.ExpiresAtMs)
	}
}

func TestV2_CreateSandbox_ReturnsMsTimestampsAndCapabilities(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	body, _ := json.Marshal(v2.CreateSandboxRequest{
		Template:     "nodejs",
		Ports:        []int{3000},
		TimeoutMs:    5_000,
		Metadata:     map[string]string{"owner": "alice"},
		Capabilities: []string{"sudo"},
	})
	req := httptest.NewRequest("POST", "/api/v2/sandboxes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp v2.SandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if resp.Template != "nodejs" {
		t.Errorf("template: got %q", resp.Template)
	}
	if resp.CreatedAtMs <= 0 || resp.ExpiresAtMs <= 0 {
		t.Errorf("expected positive ms timestamps: created=%d expires=%d", resp.CreatedAtMs, resp.ExpiresAtMs)
	}
	// 5s timeout request → expires should be roughly 5000ms after created.
	gap := resp.ExpiresAtMs - resp.CreatedAtMs
	if gap < 4_500 || gap > 5_500 {
		t.Errorf("expected expires-created ≈ 5000ms, got %dms", gap)
	}
	if len(resp.Capabilities) != 1 || resp.Capabilities[0] != "sudo" {
		t.Errorf("expected capabilities [sudo], got %v", resp.Capabilities)
	}
	if resp.Metadata["owner"] != "alice" {
		t.Errorf("expected metadata.owner=alice, got %v", resp.Metadata)
	}
	// No warm pool in the test server — always a fresh create.
	if resp.FromWarmPool {
		t.Error("FromWarmPool should be false when no warm pool is configured")
	}
}

func TestV2_GetSandbox_NotFound_StructuredError(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v2/sandboxes/does-not-exist", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: expected 404, got %d", rec.Code)
	}
	var resp v2.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != v2.CodeSandboxNotFound {
		t.Errorf("code: expected %s, got %s", v2.CodeSandboxNotFound, resp.Code)
	}
	if resp.Retryable {
		t.Error("SANDBOX_NOT_FOUND should not be retryable")
	}
	if got, _ := resp.Details["sandbox_id"].(string); got != "does-not-exist" {
		t.Errorf("details.sandbox_id: expected 'does-not-exist', got %v", resp.Details["sandbox_id"])
	}
}

func TestV2_CreateSandbox_InvalidCapability_ReturnsStructuredError(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	body, _ := json.Marshal(v2.CreateSandboxRequest{
		Template:     "base",
		Capabilities: []string{"rootkit"},
	})
	req := httptest.NewRequest("POST", "/api/v2/sandboxes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: expected 400, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp v2.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Code != v2.CodeInvalidParameter {
		t.Errorf("code: expected %s, got %s", v2.CodeInvalidParameter, resp.Code)
	}
	if got, _ := resp.Details["field"].(string); got != "capabilities" {
		t.Errorf("details.field: expected 'capabilities', got %v", resp.Details["field"])
	}
	// The allowed list should be present for LLMs to self-correct.
	allowed, ok := resp.Details["allowed"].([]any)
	if !ok {
		t.Fatalf("details.allowed missing or wrong type: %T", resp.Details["allowed"])
	}
	if len(allowed) != 3 {
		t.Errorf("expected 3 allowed values, got %d", len(allowed))
	}
}

func TestV2_ListSandboxes_ReturnsV2Shape(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v2/sandboxes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var result []v2.SandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d", len(result))
	}
}

func TestV2_V1Paths_StillUseV1ErrorShape(t *testing.T) {
	// Regression: v1 callers must continue to receive {error, code, details-as-string}.
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/sandboxes/does-not-exist", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: expected 404, got %d", rec.Code)
	}
	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["error"].(string); !ok {
		t.Errorf("v1 body must carry string `error` field, got %#v", raw)
	}
	// v2-only fields must NOT be present in v1 responses.
	if _, ok := raw["message"]; ok {
		t.Errorf("v1 body should not carry `message`, got %#v", raw)
	}
	if _, ok := raw["retryable"]; ok {
		t.Errorf("v1 body should not carry `retryable`, got %#v", raw)
	}
}
