package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/xgen-sandbox/agent/api/v1"
)

func TestAdminSummary(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/admin/summary", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var resp v1.AdminSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ActiveSandboxes != 0 {
		t.Errorf("expected 0 active sandboxes, got %d", resp.ActiveSandboxes)
	}
}

func TestAdminMetrics(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/admin/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	var resp v1.AdminMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestAdminAuditLogs(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/admin/audit-logs?limit=10&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	var resp v1.AdminAuditLogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 0 {
		t.Errorf("expected 0 total entries, got %d", resp.Total)
	}
}

func TestAdminWarmPool(t *testing.T) {
	srv, authenticator := newTestServer()
	handler := srv.Handler()

	token, _, _ := authenticator.GenerateToken("test-key")
	req := httptest.NewRequest("GET", "/api/v1/admin/warm-pool", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d", rec.Code)
	}

	var resp v1.AdminWarmPoolResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestAdminEndpoints_Unauthorized(t *testing.T) {
	srv, _ := newTestServer()
	handler := srv.Handler()

	endpoints := []string{
		"/api/v1/admin/summary",
		"/api/v1/admin/metrics",
		"/api/v1/admin/audit-logs",
		"/api/v1/admin/warm-pool",
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest("GET", ep, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401, got %d", ep, rec.Code)
		}
	}
}

func TestParseIntParam(t *testing.T) {
	tests := []struct {
		query    string
		key      string
		def      int
		expected int
	}{
		{"", "limit", 50, 50},
		{"limit=10", "limit", 50, 10},
		{"limit=abc", "limit", 50, 50},
		{"limit=-1", "limit", 50, 50},
		{"offset=5", "offset", 0, 5},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/test?"+tt.query, nil)
		got := parseIntParam(req, tt.key, tt.def)
		if got != tt.expected {
			t.Errorf("parseIntParam(%q, %q, %d) = %d, want %d", tt.query, tt.key, tt.def, got, tt.expected)
		}
	}
}
