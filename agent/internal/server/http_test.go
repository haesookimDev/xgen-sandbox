package server

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/audit"
	"github.com/xgen-sandbox/agent/internal/auth"
	"github.com/xgen-sandbox/agent/internal/config"
	k8spkg "github.com/xgen-sandbox/agent/internal/k8s"
	"github.com/xgen-sandbox/agent/internal/proxy"
	"github.com/xgen-sandbox/agent/internal/sandbox"
)

func newTestServer() (*Server, *auth.Authenticator) {
	cfg := &config.Config{
		ListenAddr:       ":8080",
		PreviewDomain:    "preview.localhost:8080",
		ExternalURL:      "http://localhost:8080",
		SandboxNamespace: "test-ns",
		SidecarImage:     "sidecar:test",
		RuntimeBaseImage: "runtime:test",
		DefaultTimeout:   time.Hour,
		MaxTimeout:       24 * time.Hour,
		APIKey:           "test-key",
		JWTSecret:        "test-secret",
	}

	authenticator := auth.NewAuthenticator(cfg.APIKey, cfg.JWTSecret)
	sandboxMgr := sandbox.NewManager()
	client := fake.NewSimpleClientset()
	podMgr := k8spkg.NewPodManagerWithClient(client, cfg.SandboxNamespace, cfg.SidecarImage, cfg.RuntimeBaseImage, corev1.PullNever, nil)
	warmPool := k8spkg.NewWarmPool(podMgr, 0)
	wsProxy := proxy.NewWSProxy(sandboxMgr)
	router := proxy.NewRouter(cfg.PreviewDomain, sandboxMgr)
	logger := slog.Default()

	auditStore := audit.NewStore(100)
	srv := NewServer(cfg, logger, authenticator, sandboxMgr, podMgr, warmPool, wsProxy, router, auditStore)
	return srv, authenticator
}

func TestHealthz(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body: expected 'ok', got %q", rec.Body.String())
	}
}

func TestAuthToken_ValidKey(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	body, _ := json.Marshal(v1.AuthTokenRequest{APIKey: "test-key"})
	req := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp v1.AuthTokenResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
}

func TestAuthToken_InvalidKey(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	body, _ := json.Marshal(v1.AuthTokenRequest{APIKey: "wrong-key"})
	req := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: expected 401, got %d", rec.Code)
	}
}

func TestListSandboxes_Unauthorized(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/v1/sandboxes", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status: expected 401, got %d", rec.Code)
	}
}

func TestListSandboxes_Empty(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/sandboxes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var result []v1.SandboxResponse
	json.NewDecoder(rec.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("expected empty list, got %d items", len(result))
	}
}

func TestGetSandbox_NotFound(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/sandboxes/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestCreateSandbox(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	body, _ := json.Marshal(v1.CreateSandboxRequest{
		Template: "nodejs",
		Ports:    []int{3000},
	})
	req := httptest.NewRequest("POST", "/api/v1/sandboxes", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status: expected 201, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp v1.SandboxResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.ID == "" {
		t.Error("expected non-empty sandbox ID")
	}
	if resp.Template != "nodejs" {
		t.Errorf("template: expected nodejs, got %q", resp.Template)
	}
	if resp.Status != v1.StatusStarting {
		t.Errorf("status: expected starting, got %q", resp.Status)
	}
}

func TestDeleteSandbox_NotFound(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("DELETE", "/api/v1/sandboxes/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Delete handler doesn't check existence first, it just tries to delete the pod
	// The pod delete will fail but the handler still returns 204
	if rec.Code != http.StatusNoContent {
		t.Errorf("status: expected 204, got %d", rec.Code)
	}
}

func TestKeepalive_NotFound(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("POST", "/api/v1/sandboxes/nonexistent/keepalive", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status: expected 404, got %d", rec.Code)
	}
}

func TestApiKeyAuth(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/v1/sandboxes", nil)
	req.Header.Set("Authorization", "ApiKey test-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}
