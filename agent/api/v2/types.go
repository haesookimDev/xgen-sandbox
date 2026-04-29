package v2

// SandboxStatus mirrors v1.SandboxStatus. The wire values are kept
// identical so callers can reason about status strings regardless of
// which version they negotiate.
type SandboxStatus string

const (
	StatusStarting SandboxStatus = "starting"
	StatusRunning  SandboxStatus = "running"
	StatusStopping SandboxStatus = "stopping"
	StatusStopped  SandboxStatus = "stopped"
	StatusError    SandboxStatus = "error"
)

// ResourceSpec defines per-sandbox resource limits.
type ResourceSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk,omitempty"`
}

// CreateSandboxRequest is the request body for POST /api/v2/sandboxes.
//
// All time inputs are in milliseconds (vs. seconds in v1). This is the
// canonical unit for every timestamp and duration in v2.
type CreateSandboxRequest struct {
	Template     string            `json:"template"`
	TimeoutMs    int64             `json:"timeout_ms,omitempty"`
	Resources    *ResourceSpec     `json:"resources,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	Ports        []int             `json:"ports,omitempty"`
	GUI          bool              `json:"gui,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
}

// SandboxResponse is the response body for sandbox operations in v2.
//
// Notable differences from v1:
//   - created_at_ms / expires_at_ms are Unix epoch milliseconds (int64)
//   - capabilities is always present (v1 omitted it in the create response)
//   - from_warm_pool reports whether the sandbox was served from the
//     warm pool (used for performance observability)
type SandboxResponse struct {
	ID           string            `json:"id"`
	Status       SandboxStatus     `json:"status"`
	Template     string            `json:"template"`
	WsURL        string            `json:"ws_url"`
	PreviewURLs  map[int]string    `json:"preview_urls,omitempty"`
	VncURL       *string           `json:"vnc_url,omitempty"`
	CreatedAtMs  int64             `json:"created_at_ms"`
	ExpiresAtMs  int64             `json:"expires_at_ms"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	FromWarmPool bool              `json:"from_warm_pool"`
}

// ExecRequest is the request body for POST /api/v2/sandboxes/:id/exec.
type ExecRequest struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	TimeoutMs      int64             `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
	MaxStdoutBytes int               `json:"max_stdout_bytes,omitempty"`
	MaxStderrBytes int               `json:"max_stderr_bytes,omitempty"`
	ArtifactPath   string            `json:"artifact_path,omitempty"`
}

// ExecResponse is the response body for exec operations in v2.
type ExecResponse struct {
	ExitCode         int     `json:"exit_code"`
	Stdout           string  `json:"stdout"`
	Stderr           string  `json:"stderr"`
	Truncated        bool    `json:"truncated"`
	StdoutTruncated  bool    `json:"stdout_truncated,omitempty"`
	StderrTruncated  bool    `json:"stderr_truncated,omitempty"`
	TruncationMarker string  `json:"truncation_marker,omitempty"`
	ArtifactPath     *string `json:"artifact_path,omitempty"`
}

// AuthTokenRequest is the request body for POST /api/v2/auth/token.
type AuthTokenRequest struct {
	APIKey string `json:"api_key"`
}

// AuthTokenResponse is the response body for token exchange in v2.
type AuthTokenResponse struct {
	Token       string `json:"token"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
}

// ServiceInfo describes one exposed port on a sandbox.
type ServiceInfo struct {
	Port       int    `json:"port"`
	PreviewURL string `json:"preview_url"`
}
