package v1

import "time"

// SandboxStatus represents the lifecycle state of a sandbox.
type SandboxStatus string

const (
	StatusStarting SandboxStatus = "starting"
	StatusRunning  SandboxStatus = "running"
	StatusStopping SandboxStatus = "stopping"
	StatusStopped  SandboxStatus = "stopped"
	StatusError    SandboxStatus = "error"
)

// ResourceSpec defines resource limits for a sandbox.
type ResourceSpec struct {
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk,omitempty"`
}

// CreateSandboxRequest is the request body for POST /api/v1/sandboxes.
type CreateSandboxRequest struct {
	Template  string            `json:"template"`
	Timeout   int               `json:"timeout_seconds,omitempty"`
	Resources *ResourceSpec     `json:"resources,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Ports     []int             `json:"ports,omitempty"`
	GUI       bool              `json:"gui,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// SandboxResponse is the response body for sandbox operations.
type SandboxResponse struct {
	ID          string            `json:"id"`
	Status      SandboxStatus     `json:"status"`
	Template    string            `json:"template"`
	WsURL       string            `json:"ws_url"`
	PreviewURLs map[int]string    `json:"preview_urls,omitempty"`
	VncURL      *string           `json:"vnc_url,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ExecRequest is the request body for POST /api/v1/sandboxes/:id/exec.
type ExecRequest struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Timeout int               `json:"timeout_seconds,omitempty"`
}

// ExecResponse is the response body for exec operations.
type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// ErrorResponse is a generic error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Details string `json:"details,omitempty"`
}

// AuthTokenRequest is the request body for POST /api/v1/auth/token.
type AuthTokenRequest struct {
	APIKey string `json:"api_key"`
}

// AuthTokenResponse is the response body for token exchange.
type AuthTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}
