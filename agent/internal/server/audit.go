package server

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/xgen-sandbox/agent/internal/audit"
	"github.com/xgen-sandbox/agent/internal/auth"
)

// auditLog logs security-relevant API actions (create, delete, exec) with user identity.
// It also records entries to the audit store for dashboard querying.
func auditLog(logger *slog.Logger, store *audit.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := &statusWriter{ResponseWriter: w, status: 200}
			next.ServeHTTP(ww, r)

			// Only audit mutating or sensitive operations
			if r.Method == http.MethodGet && r.URL.Path != "/api/v1/sandboxes/{id}/ws" {
				return
			}

			claims := auth.GetClaims(r.Context())
			subject := "anonymous"
			role := ""
			if claims != nil {
				subject = claims.Subject
				role = string(claims.Role)
			}

			action := r.Method + " " + r.URL.Path

			logger.Info("audit",
				slog.String("action", action),
				slog.String("subject", subject),
				slog.String("role", role),
				slog.Int("status", ww.status),
				slog.String("remote", r.RemoteAddr),
			)

			// Extract sandbox ID from URL path if present
			sandboxID := extractSandboxID(r.URL.Path)

			store.Add(audit.Entry{
				Timestamp: time.Now(),
				Action:    action,
				Subject:   subject,
				Role:      role,
				Status:    ww.status,
				RemoteIP:  r.RemoteAddr,
				SandboxID: sandboxID,
			})
		})
	}
}

// extractSandboxID extracts the sandbox ID from URL paths like /api/v1/sandboxes/{id}/...
func extractSandboxID(path string) string {
	const prefix = "/api/v1/sandboxes/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	if idx := strings.Index(rest, "/"); idx != -1 {
		return rest[:idx]
	}
	return rest
}
