package v2

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTTPStatusForAllCodes ensures every registered code has a sensible
// HTTP status mapping. Unknown codes fall through to 500.
func TestHTTPStatusForAllCodes(t *testing.T) {
	for _, c := range AllCodes() {
		if got := HTTPStatus(c); got < 400 || got > 599 {
			t.Errorf("code %s: HTTPStatus() = %d, want 4xx or 5xx", c, got)
		}
	}

	// Sanity check on a handful of explicit mappings to guard against reordering.
	want := map[ErrorCode]int{
		CodeInvalidRequest:   http.StatusBadRequest,
		CodeUnauthorized:     http.StatusUnauthorized,
		CodeSandboxNotFound:  http.StatusNotFound,
		CodeQuotaExceeded:    http.StatusTooManyRequests,
		CodeInternal:         http.StatusInternalServerError,
		CodeExecTimeout:      http.StatusGatewayTimeout,
	}
	for code, expected := range want {
		if got := HTTPStatus(code); got != expected {
			t.Errorf("HTTPStatus(%s) = %d, want %d", code, got, expected)
		}
	}
}

// TestUnknownCodeFallback locks in the contract for codes we have not
// registered yet — HTTP 500, not retryable, generic message.
func TestUnknownCodeFallback(t *testing.T) {
	const c ErrorCode = "DOES_NOT_EXIST"
	if got := HTTPStatus(c); got != http.StatusInternalServerError {
		t.Errorf("unknown code HTTPStatus = %d, want 500", got)
	}
	if Retryable(c) {
		t.Error("unknown code should not be retryable by default")
	}
	if msg := DefaultMessage(c); msg == "" {
		t.Error("unknown code DefaultMessage should not be empty")
	}
}

// TestCodeFormat enforces the naming convention (SCREAMING_SNAKE_CASE, ASCII).
func TestCodeFormat(t *testing.T) {
	for _, c := range AllCodes() {
		s := string(c)
		if s == "" {
			t.Error("empty code in registry")
		}
		for _, r := range s {
			if !(r == '_' || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
				t.Errorf("code %q contains invalid rune %q; must be SCREAMING_SNAKE_CASE", s, r)
				break
			}
		}
	}
}

// TestRegistryMatchesDocs ensures docs/error-codes.md lists every registered
// code at least once. It prevents the registry and docs from drifting.
func TestRegistryMatchesDocs(t *testing.T) {
	docsPath := filepath.Join("..", "..", "..", "docs", "error-codes.md")
	data, err := os.ReadFile(docsPath)
	if err != nil {
		t.Skipf("docs file not found at %s (skipping docs-sync check): %v", docsPath, err)
		return
	}
	body := string(data)
	for _, c := range AllCodes() {
		needle := "`" + string(c) + "`"
		if !strings.Contains(body, needle) {
			t.Errorf("docs/error-codes.md missing registered code %s", c)
		}
	}
}
