// Package v2 defines the v2 HTTP/WebSocket API surface.
//
// The v2 error model is structured: every error response carries a stable
// machine-readable Code, a human-readable Message, optional Details object,
// the request ID for log correlation, and a Retryable hint.
//
// v1 continues to work. writeAPIError (see server package) serialises the
// same error into the v1 legacy shape for /api/v1/* paths.
package v2

import "net/http"

// ErrorCode is a stable, machine-readable error identifier.
//
// Naming: SCREAMING_SNAKE_CASE. Every code MUST also appear in
// docs/error-codes.md (enforced by errors_test.go in this package).
type ErrorCode string

const (
	// CodeInvalidRequest — request body could not be parsed.
	CodeInvalidRequest ErrorCode = "INVALID_REQUEST"
	// CodeInvalidParameter — a parameter violated validation rules.
	// Details typically include "field" and "allowed" or "min"/"max".
	CodeInvalidParameter ErrorCode = "INVALID_PARAMETER"
	// CodeUnauthorized — missing or invalid credentials.
	CodeUnauthorized ErrorCode = "UNAUTHORIZED"
	// CodeForbidden — authenticated but lacks the required permission.
	CodeForbidden ErrorCode = "FORBIDDEN"
	// CodeSandboxNotFound — the sandbox id does not exist.
	CodeSandboxNotFound ErrorCode = "SANDBOX_NOT_FOUND"
	// CodeSandboxNotReady — the sandbox exists but has no pod IP yet.
	CodeSandboxNotReady ErrorCode = "SANDBOX_NOT_READY"
	// CodeSandboxExpired — the sandbox is past its expiry and has been removed.
	CodeSandboxExpired ErrorCode = "SANDBOX_EXPIRED"
	// CodeQuotaExceeded — rate-limit or per-tenant quota hit.
	// Details may include "retry_after_ms".
	CodeQuotaExceeded ErrorCode = "QUOTA_EXCEEDED"
	// CodePodCreateFailed — the Kubernetes pod could not be created.
	// Details.reason carries a short internal explanation.
	CodePodCreateFailed ErrorCode = "POD_CREATE_FAILED"
	// CodeSidecarUnreachable — the agent could not reach the sidecar WS.
	CodeSidecarUnreachable ErrorCode = "SIDECAR_UNREACHABLE"
	// CodeExecTimeout — REST /exec timed out waiting for ExecExit.
	CodeExecTimeout ErrorCode = "EXEC_TIMEOUT"
	// CodeInternal — unclassified internal failure.
	CodeInternal ErrorCode = "INTERNAL"
)

// ErrorResponse is the v2 error body.
type ErrorResponse struct {
	Code      ErrorCode      `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Retryable bool           `json:"retryable"`
}

// registry holds the per-code metadata. Order is preserved for docs generation
// and registry tests. Adding a new ErrorCode requires appending an entry here
// AND adding a row to docs/error-codes.md.
var registry = []struct {
	Code       ErrorCode
	HTTPStatus int
	Retryable  bool
	Summary    string
}{
	{CodeInvalidRequest, http.StatusBadRequest, false, "Request body could not be parsed"},
	{CodeInvalidParameter, http.StatusBadRequest, false, "A parameter failed validation"},
	{CodeUnauthorized, http.StatusUnauthorized, false, "Missing or invalid credentials"},
	{CodeForbidden, http.StatusForbidden, false, "Authenticated but lacks permission"},
	{CodeSandboxNotFound, http.StatusNotFound, false, "Sandbox id does not exist"},
	{CodeSandboxNotReady, http.StatusConflict, true, "Sandbox has no pod IP yet"},
	{CodeSandboxExpired, http.StatusGone, false, "Sandbox past expiry and removed"},
	{CodeQuotaExceeded, http.StatusTooManyRequests, true, "Rate limit or quota hit"},
	{CodePodCreateFailed, http.StatusServiceUnavailable, true, "Kubernetes pod creation failed"},
	{CodeSidecarUnreachable, http.StatusServiceUnavailable, true, "Sidecar WebSocket unreachable"},
	{CodeExecTimeout, http.StatusGatewayTimeout, false, "Exec did not produce exit within timeout"},
	{CodeInternal, http.StatusInternalServerError, true, "Unclassified internal error"},
}

// byCode is built lazily for O(1) lookups. Population happens in init so tests
// can rely on a fully populated map.
var byCode map[ErrorCode]int

func init() {
	byCode = make(map[ErrorCode]int, len(registry))
	for i, e := range registry {
		byCode[e.Code] = i
	}
}

// HTTPStatus returns the recommended HTTP status for a code.
// Unknown codes fall back to 500.
func HTTPStatus(code ErrorCode) int {
	if i, ok := byCode[code]; ok {
		return registry[i].HTTPStatus
	}
	return http.StatusInternalServerError
}

// Retryable reports whether the caller should retry after this error.
// Unknown codes are treated as non-retryable.
func Retryable(code ErrorCode) bool {
	if i, ok := byCode[code]; ok {
		return registry[i].Retryable
	}
	return false
}

// DefaultMessage returns a terse human summary for a code.
// Callers typically pass a more specific message; this is the fallback.
func DefaultMessage(code ErrorCode) string {
	if i, ok := byCode[code]; ok {
		return registry[i].Summary
	}
	return "Internal error"
}

// AllCodes returns every registered code in registration order.
// Intended for docs generation and tests.
func AllCodes() []ErrorCode {
	out := make([]ErrorCode, len(registry))
	for i, e := range registry {
		out[i] = e.Code
	}
	return out
}
