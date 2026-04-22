package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	v1 "github.com/xgen-sandbox/agent/api/v1"
)

func TestDeprecation_V1AuthTokenCarriesHeaders(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	body, _ := json.Marshal(v1.AuthTokenRequest{APIKey: "test-key"})
	req := httptest.NewRequest("POST", "/api/v1/auth/token", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}
	assertDeprecationHeaders(t, rec.Header())
}

func TestDeprecation_V1ProtectedRouteCarriesHeaders(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/sandboxes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}
	assertDeprecationHeaders(t, rec.Header())
}

func TestDeprecation_V1ErrorResponseCarriesHeaders(t *testing.T) {
	// Regression: headers must be set even when the handler short-circuits
	// with an error. writeAPIError does not clobber pre-set headers.
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/sandboxes/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: expected 404, got %d", rec.Code)
	}
	assertDeprecationHeaders(t, rec.Header())
}

func TestDeprecation_V2ResponseHasNoDeprecationHeaders(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v2/sandboxes", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Deprecation") != "" {
		t.Errorf("v2 must not carry Deprecation header, got %q", rec.Header().Get("Deprecation"))
	}
	if rec.Header().Get("Sunset") != "" {
		t.Errorf("v2 must not carry Sunset header, got %q", rec.Header().Get("Sunset"))
	}
}

func TestDeprecation_V1RequestsMetricBumpsWithRoutePattern(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	const routePattern = "/api/v1/sandboxes/{id}"
	before := testutil.ToFloat64(apiV1RequestsTotal.WithLabelValues(routePattern))

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/sandboxes/anything", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	after := testutil.ToFloat64(apiV1RequestsTotal.WithLabelValues(routePattern))
	if delta := after - before; delta != 1 {
		t.Errorf("expected counter delta of 1 for %s, got %v (before=%v after=%v)",
			routePattern, delta, before, after)
	}

	// Cardinality guard: the sandbox id must not leak into a label.
	leaked := testutil.ToFloat64(apiV1RequestsTotal.WithLabelValues("/api/v1/sandboxes/anything"))
	if leaked != 0 {
		t.Errorf("sandbox id leaked into metric label, got %v for raw path", leaked)
	}
}

func assertDeprecationHeaders(t *testing.T, h http.Header) {
	t.Helper()
	if got := h.Get("Deprecation"); got != "true" {
		t.Errorf("Deprecation: expected 'true', got %q", got)
	}
	if got := h.Get("Sunset"); got == "" {
		t.Error("Sunset header missing")
	}
	if got := h.Get("Warning"); !strings.HasPrefix(got, "299 ") {
		t.Errorf("Warning: expected to start with '299 ', got %q", got)
	}
	if got := h.Get("Link"); !strings.Contains(got, `rel="deprecation"`) {
		t.Errorf("Link: expected rel=\"deprecation\", got %q", got)
	}
}
