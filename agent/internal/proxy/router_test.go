package proxy

import (
	"net/http"
	"testing"
)

func TestParseSubdomain_Valid(t *testing.T) {
	id, port, err := parseSubdomain("sbx-abc123def-3000")
	if err != nil {
		t.Fatalf("parseSubdomain() error: %v", err)
	}
	if id != "abc123def" {
		t.Errorf("sandboxID: expected abc123def, got %q", id)
	}
	if port != "3000" {
		t.Errorf("port: expected 3000, got %q", port)
	}
}

func TestParseSubdomain_MultipleDashes(t *testing.T) {
	id, port, err := parseSubdomain("sbx-abc-def-ghi-8080")
	if err != nil {
		t.Fatalf("parseSubdomain() error: %v", err)
	}
	if id != "abc-def-ghi" {
		t.Errorf("sandboxID: expected abc-def-ghi, got %q", id)
	}
	if port != "8080" {
		t.Errorf("port: expected 8080, got %q", port)
	}
}

func TestParseSubdomain_NoPrefix(t *testing.T) {
	_, _, err := parseSubdomain("xyz-abc-3000")
	if err == nil {
		t.Error("expected error for missing sbx- prefix")
	}
}

func TestParseSubdomain_NoDash(t *testing.T) {
	_, _, err := parseSubdomain("sbx-nodash")
	if err == nil {
		t.Error("expected error for missing port separator")
	}
}

func TestParseSubdomain_NonNumericPort(t *testing.T) {
	// Regression: previously accepted and failed later at dial time.
	if _, _, err := parseSubdomain("sbx-abc123-foo"); err == nil {
		t.Error("expected error for non-numeric port")
	}
}

func TestParseSubdomain_OutOfRangePort(t *testing.T) {
	for _, port := range []string{"0", "65536", "99999"} {
		if _, _, err := parseSubdomain("sbx-abc-" + port); err == nil {
			t.Errorf("expected error for port %s", port)
		}
	}
}

func TestParseSubdomain_EmptySandboxID(t *testing.T) {
	if _, _, err := parseSubdomain("sbx--3000"); err == nil {
		t.Error("expected error for empty sandbox id")
	}
}

func TestNewRouter_StripsDomainPort(t *testing.T) {
	r := NewRouter("preview.localhost:8080", nil)
	if r.DomainHost() != "preview.localhost" {
		t.Errorf("DomainHost: expected preview.localhost, got %q", r.DomainHost())
	}
}

func TestNewRouter_NoDomainPort(t *testing.T) {
	r := NewRouter("preview.example.com", nil)
	if r.DomainHost() != "preview.example.com" {
		t.Errorf("DomainHost: expected preview.example.com, got %q", r.DomainHost())
	}
}

func TestPreviewURL_LocalhostHTTP(t *testing.T) {
	r := NewRouter("preview.localhost:8080", nil)
	url := r.PreviewURL("abc123", 3000)
	expected := "http://sbx-abc123-3000.preview.localhost:8080"
	if url != expected {
		t.Errorf("PreviewURL: expected %q, got %q", expected, url)
	}
}

func TestPreviewURL_ProductionHTTPS(t *testing.T) {
	r := NewRouter("preview.example.com", nil)
	url := r.PreviewURL("abc123", 3000)
	expected := "https://sbx-abc123-3000.preview.example.com"
	if url != expected {
		t.Errorf("PreviewURL: expected %q, got %q", expected, url)
	}
}

func TestIsWebSocketUpgrade_True(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Upgrade", "websocket")
	if !isWebSocketUpgrade(req) {
		t.Error("expected true for websocket upgrade header")
	}
}

func TestIsWebSocketUpgrade_CaseInsensitive(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Upgrade", "WebSocket")
	if !isWebSocketUpgrade(req) {
		t.Error("expected true for case-insensitive websocket")
	}
}

func TestIsWebSocketUpgrade_False(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	if isWebSocketUpgrade(req) {
		t.Error("expected false when no upgrade header")
	}
}

func TestIsWebSocketUpgrade_OtherProtocol(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Upgrade", "h2c")
	if isWebSocketUpgrade(req) {
		t.Error("expected false for non-websocket upgrade")
	}
}
