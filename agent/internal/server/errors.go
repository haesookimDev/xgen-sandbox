package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	v2 "github.com/xgen-sandbox/agent/api/v2"
)

// writeAPIError emits a version-aware error response.
//
// For /api/v2/* paths the body is v2.ErrorResponse. For all other paths
// (/api/v1/*, bare routes) the body stays in the v1 shape so existing
// clients do not break; the v2 code is carried in the `code` field and
// structured `details` are serialised as a compact JSON string in
// v1.ErrorResponse.Details.
//
// Pass message="" to fall back to v2.DefaultMessage(code).
// Pass details=nil when there is nothing to add.
func writeAPIError(w http.ResponseWriter, r *http.Request, code v2.ErrorCode, message string, details map[string]any) {
	if message == "" {
		message = v2.DefaultMessage(code)
	}
	status := v2.HTTPStatus(code)

	if isV2Request(r) {
		resp := v2.ErrorResponse{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: middleware.GetReqID(r.Context()),
			Retryable: v2.Retryable(code),
		}
		writeJSON(w, status, resp)
		return
	}

	// v1 compatibility shape: preserve {error, code, details} with details as string.
	v1resp := v1.ErrorResponse{
		Error: message,
		Code:  string(code),
	}
	if len(details) > 0 {
		if b, err := json.Marshal(details); err == nil {
			v1resp.Details = string(b)
		}
	}
	writeJSON(w, status, v1resp)
}

// isV2Request reports whether the request targets the v2 API namespace.
func isV2Request(r *http.Request) bool {
	return r != nil && strings.HasPrefix(r.URL.Path, "/api/v2/")
}
