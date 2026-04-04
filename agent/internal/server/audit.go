package server

import (
	"log/slog"
	"net/http"

	"github.com/xgen-sandbox/agent/internal/auth"
)

// auditLog logs security-relevant API actions (create, delete, exec) with user identity.
func auditLog(logger *slog.Logger) func(http.Handler) http.Handler {
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

			logger.Info("audit",
				slog.String("action", r.Method+" "+r.URL.Path),
				slog.String("subject", subject),
				slog.String("role", role),
				slog.Int("status", ww.status),
				slog.String("remote", r.RemoteAddr),
			)
		})
	}
}
