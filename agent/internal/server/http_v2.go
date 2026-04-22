package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	v2 "github.com/xgen-sandbox/agent/api/v2"
	k8spkg "github.com/xgen-sandbox/agent/internal/k8s"
	"github.com/xgen-sandbox/agent/internal/sandbox"
)

// v2 HTTP handlers. These share core state (sandboxMgr, podMgr, warmPool,
// wsProxy) with v1 handlers via the Server struct. Only the on-the-wire
// shape and unit conventions differ:
//
//   - Timestamps are Unix epoch milliseconds (int64).
//   - Durations are milliseconds (int64).
//   - Errors are structured v2.ErrorResponse (writeAPIError handles the
//     per-path shape selection).
//   - Capabilities are always echoed in the create response (v1 has a
//     historical omission that is preserved for compatibility).

var validCapabilities = map[string]bool{"sudo": true, "git-ssh": true, "browser": true}

// handleAuthTokenV2 mirrors the v1 token handler but returns ExpiresAtMs.
func (s *Server) handleAuthTokenV2(w http.ResponseWriter, r *http.Request) {
	var req v2.AuthTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, r, v2.CodeInvalidRequest, "invalid request body", nil)
		return
	}
	token, expiresAt, err := s.auth.GenerateToken(req.APIKey)
	if err != nil {
		AuthFailuresTotal.WithLabelValues("invalid_api_key").Inc()
		writeAPIError(w, r, v2.CodeUnauthorized, err.Error(), nil)
		return
	}
	AuthTokenGeneratedTotal.Inc()
	writeJSON(w, http.StatusOK, v2.AuthTokenResponse{
		Token:       token,
		ExpiresAtMs: expiresAt.UnixMilli(),
	})
}

func (s *Server) handleCreateSandboxV2(w http.ResponseWriter, r *http.Request) {
	var req v2.CreateSandboxRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, r, v2.CodeInvalidRequest, "invalid request body", nil)
		return
	}

	if req.Template == "" {
		req.Template = "base"
	}

	if err := validatePorts(req.Ports); err != nil {
		writeAPIErrorFromValidation(w, r, err)
		return
	}
	if err := validateMetadata(req.Metadata); err != nil {
		writeAPIErrorFromValidation(w, r, err)
		return
	}
	caps, err := validateCapabilities(req.Capabilities)
	if err != nil {
		writeAPIErrorFromValidation(w, r, err)
		return
	}
	req.Capabilities = caps

	// browser implies gui + min resources.
	if containsString(caps, "browser") {
		req.GUI = true
		if req.Resources == nil {
			req.Resources = &v2.ResourceSpec{CPU: "2000m", Memory: "2Gi"}
		}
	}

	timeout := s.cfg.DefaultTimeout
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
		if timeout > s.cfg.MaxTimeout {
			timeout = s.cfg.MaxTimeout
		}
	}

	sbx := s.sandboxMgr.Create(req.Template, timeout, req.Ports, req.GUI, req.Env, req.Metadata, req.Capabilities)
	sandboxCreateTotal.Inc()
	sandboxesActive.Inc()
	SandboxesByTemplate.WithLabelValues(req.Template).Inc()
	SandboxesByStatus.WithLabelValues(string(v1.StatusStarting)).Inc()

	var rs *k8spkg.ResourceSpec
	if req.Resources != nil {
		rs = &k8spkg.ResourceSpec{CPU: req.Resources.CPU, Memory: req.Resources.Memory}
	}
	fromWarm, err := s.provisionSandboxPod(r.Context(), sbx, req.Env, req.Ports, req.GUI, req.Capabilities, rs)
	if err != nil {
		writeAPIError(w, r, v2.CodePodCreateFailed, "failed to create sandbox",
			map[string]any{"reason": err.Error(), "template": req.Template})
		return
	}

	writeJSON(w, http.StatusCreated, s.sandboxToResponseV2(sbx, fromWarm))
}

func (s *Server) handleListSandboxesV2(w http.ResponseWriter, r *http.Request) {
	sandboxes := s.sandboxMgr.List()
	result := make([]v2.SandboxResponse, 0, len(sandboxes))
	for _, sbx := range sandboxes {
		result = append(result, s.sandboxToResponseV2(sbx, false))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleGetSandboxV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sbx, err := s.sandboxMgr.Get(id)
	if err != nil {
		writeAPIError(w, r, v2.CodeSandboxNotFound, "sandbox not found",
			map[string]any{"sandbox_id": id})
		return
	}
	writeJSON(w, http.StatusOK, s.sandboxToResponseV2(sbx, false))
}

func (s *Server) handleDeleteSandboxV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := s.podMgr.DeletePod(r.Context(), id); err != nil {
		if !apierrors.IsNotFound(err) {
			writeAPIError(w, r, v2.CodeInternal, "failed to delete sandbox pod",
				map[string]any{"sandbox_id": id, "reason": err.Error()})
			return
		}
	}

	if sbx, err := s.sandboxMgr.Get(id); err == nil {
		SandboxesByTemplate.WithLabelValues(sbx.Template).Dec()
		SandboxesByStatus.WithLabelValues(string(sbx.Status)).Dec()
	}

	s.wsProxy.DisconnectSidecar(id)
	s.sandboxMgr.SetStatus(id, v1.StatusStopped)
	s.sandboxMgr.Remove(id)
	sandboxDeleteTotal.Inc()
	sandboxesActive.Dec()

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleKeepaliveV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.sandboxMgr.ExtendTimeout(id, s.cfg.DefaultTimeout); err != nil {
		writeAPIError(w, r, v2.CodeSandboxNotFound, "sandbox not found",
			map[string]any{"sandbox_id": id})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleExecV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.sandboxMgr.Get(id); err != nil {
		writeAPIError(w, r, v2.CodeSandboxNotFound, "sandbox not found",
			map[string]any{"sandbox_id": id})
		return
	}

	var req v2.ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, r, v2.CodeInvalidRequest, "invalid request body", nil)
		return
	}
	if req.Command == "" {
		writeAPIError(w, r, v2.CodeInvalidParameter, "command is required",
			map[string]any{"field": "command"})
		return
	}

	timeout := 30 * time.Second
	if req.TimeoutMs > 0 {
		timeout = time.Duration(req.TimeoutMs) * time.Millisecond
	}

	result, err := s.wsProxy.ExecSync(r.Context(), id, req.Command, req.Args, req.Env, req.Cwd, timeout)
	if err != nil {
		code := v2.CodeInternal
		msg := err.Error()
		low := strings.ToLower(msg)
		switch {
		case strings.Contains(low, "timeout"), strings.Contains(low, "deadline"):
			code = v2.CodeExecTimeout
		case strings.Contains(low, "ws connect"), strings.Contains(low, "sidecar"), strings.Contains(low, "connection"):
			code = v2.CodeSidecarUnreachable
		}
		writeAPIError(w, r, code, msg, map[string]any{"sandbox_id": id})
		return
	}

	writeJSON(w, http.StatusOK, v2.ExecResponse{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	})
}

func (s *Server) handleListServicesV2(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sbx, err := s.sandboxMgr.Get(id)
	if err != nil {
		writeAPIError(w, r, v2.CodeSandboxNotFound, "sandbox not found",
			map[string]any{"sandbox_id": id})
		return
	}
	services := make([]v2.ServiceInfo, 0, len(sbx.Ports))
	for _, port := range sbx.Ports {
		services = append(services, v2.ServiceInfo{
			Port:       port,
			PreviewURL: s.router.PreviewURL(sbx.ID, port),
		})
	}
	writeJSON(w, http.StatusOK, services)
}

// sandboxToResponseV2 converts an internal sandbox to the v2 wire shape.
// fromWarmPool is the truth known only at create time; List/Get default to
// false because we don't persist the origin.
func (s *Server) sandboxToResponseV2(sbx *sandbox.Sandbox, fromWarmPool bool) v2.SandboxResponse {
	previewURLs := make(map[int]string)
	for _, port := range sbx.Ports {
		previewURLs[port] = s.router.PreviewURL(sbx.ID, port)
	}

	resp := v2.SandboxResponse{
		ID:           sbx.ID,
		Status:       v2.SandboxStatus(sbx.Status),
		Template:     sbx.Template,
		WsURL:        fmt.Sprintf("%s/api/v2/sandboxes/%s/ws", s.cfg.ExternalURL, sbx.ID),
		PreviewURLs:  previewURLs,
		CreatedAtMs:  sbx.CreatedAt.UnixMilli(),
		ExpiresAtMs:  sbx.ExpiresAt.UnixMilli(),
		Metadata:     sbx.Metadata,
		Capabilities: sbx.Capabilities,
		FromWarmPool: fromWarmPool,
	}
	if sbx.GUI {
		vncURL := s.router.PreviewURL(sbx.ID, 6080)
		resp.VncURL = &vncURL
	}
	return resp
}

// --- Validation helpers shared by v1/v2 create ---

// validationError carries a structured error along with the ErrorCode/details
// that should be emitted. Keeps handlers slim.
type validationError struct {
	code    v2.ErrorCode
	message string
	details map[string]any
}

func (e *validationError) Error() string { return e.message }

func writeAPIErrorFromValidation(w http.ResponseWriter, r *http.Request, err error) {
	if ve, ok := err.(*validationError); ok {
		writeAPIError(w, r, ve.code, ve.message, ve.details)
		return
	}
	writeAPIError(w, r, v2.CodeInternal, err.Error(), nil)
}

func validatePorts(ports []int) error {
	seen := make(map[int]bool)
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return &validationError{
				code:    v2.CodeInvalidParameter,
				message: fmt.Sprintf("invalid port: %d (must be 1-65535)", port),
				details: map[string]any{"field": "ports", "value": port, "min": 1, "max": 65535},
			}
		}
		if seen[port] {
			return &validationError{
				code:    v2.CodeInvalidParameter,
				message: fmt.Sprintf("duplicate port: %d", port),
				details: map[string]any{"field": "ports", "value": port, "reason": "duplicate"},
			}
		}
		seen[port] = true
	}
	return nil
}

func validateMetadata(md map[string]string) error {
	if len(md) > 20 {
		return &validationError{
			code:    v2.CodeInvalidParameter,
			message: "too many metadata entries (max 20)",
			details: map[string]any{"field": "metadata", "max_entries": 20, "count": len(md)},
		}
	}
	for k, v := range md {
		if len(k) > 128 || len(v) > 1024 {
			return &validationError{
				code:    v2.CodeInvalidParameter,
				message: "metadata key max 128 chars, value max 1024 chars",
				details: map[string]any{"field": "metadata", "max_key_chars": 128, "max_value_chars": 1024},
			}
		}
	}
	return nil
}

// validateCapabilities returns the deduplicated capability list or a
// validation error for an unknown capability.
func validateCapabilities(caps []string) ([]string, error) {
	seen := make(map[string]bool)
	for _, c := range caps {
		if !validCapabilities[c] {
			return nil, &validationError{
				code:    v2.CodeInvalidParameter,
				message: fmt.Sprintf("unknown capability: %q", c),
				details: map[string]any{"field": "capabilities", "value": c, "allowed": []string{"sudo", "git-ssh", "browser"}},
			}
		}
		seen[c] = true
	}
	if len(seen) == len(caps) {
		return caps, nil
	}
	deduped := make([]string, 0, len(seen))
	for c := range seen {
		deduped = append(deduped, c)
	}
	return deduped, nil
}

func containsString(slice []string, want string) bool {
	for _, s := range slice {
		if s == want {
			return true
		}
	}
	return false
}
