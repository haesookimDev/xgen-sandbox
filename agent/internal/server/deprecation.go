package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// v1SunsetDate is the announced cut-off for v1 support.
//
// Announced in docs/migration-v1-v2.md. Clients receiving the Sunset
// header should migrate before this date; after the sunset v1 may
// return 410 Gone.
const v1SunsetDate = "Sun, 01 Nov 2026 00:00:00 GMT"

// v1WarningMessage follows RFC 7234 §5.5 Warning header syntax
// (299 = "Miscellaneous persistent warning").
const v1WarningMessage = `299 - "API v1 is deprecated, migrate to /api/v2 by 2026-11-01"`

// v1DeprecationLink points to the migration guide. Emitted via the
// standard `Link: ...; rel="deprecation"` header (draft-ietf-httpapi-deprecation).
const v1DeprecationLink = `</docs/migration-v1-v2.md>; rel="deprecation"`

// deprecationMiddleware annotates v1 responses with the canonical
// deprecation headers and bumps a Prometheus counter labelled by
// route pattern so operators can watch v1 traffic drain.
//
// Headers follow IETF draft-ietf-httpapi-deprecation-header:
//   Deprecation: true
//   Sunset:      <HTTP-date>
//   Link:        <...>; rel="deprecation"
//   Warning:     299 - "..."
//
// The counter is bumped after the handler runs so chi's matched route
// pattern is available — this keeps label cardinality bounded (no
// sandbox IDs leak into metric labels).
func deprecationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Deprecation", "true")
		h.Set("Sunset", v1SunsetDate)
		h.Set("Warning", v1WarningMessage)
		h.Set("Link", v1DeprecationLink)

		next.ServeHTTP(w, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unknown"
		}
		apiV1RequestsTotal.WithLabelValues(route).Inc()
	})
}
