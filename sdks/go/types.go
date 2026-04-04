package xgen

// SandboxStatus represents the lifecycle state of a sandbox.
type SandboxStatus string

const (
	StatusStarting SandboxStatus = "starting"
	StatusRunning  SandboxStatus = "running"
	StatusStopping SandboxStatus = "stopping"
	StatusStopped  SandboxStatus = "stopped"
	StatusError    SandboxStatus = "error"
)

// CreateSandboxOptions configures a new sandbox.
type CreateSandboxOptions struct {
	Template       string            `json:"template,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Resources      *Resources        `json:"resources,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Ports          []int             `json:"ports,omitempty"`
	GUI            bool              `json:"gui,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

// Resources specifies sandbox resource limits.
type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
}

// SandboxInfo holds metadata about a sandbox returned by the API.
type SandboxInfo struct {
	ID          string            `json:"id"`
	Status      SandboxStatus     `json:"status"`
	Template    string            `json:"template"`
	WsURL       string            `json:"ws_url"`
	PreviewURLs map[int]string    `json:"preview_urls"`
	VncURL      string            `json:"vnc_url,omitempty"`
	CreatedAt   string            `json:"created_at"`
	ExpiresAt   string            `json:"expires_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ExecOption is a functional option for Exec.
type ExecOption func(*execConfig)

type execConfig struct {
	Args    []string
	Env     map[string]string
	Cwd     string
	Timeout int // seconds
}

// WithArgs sets additional arguments for the command.
func WithArgs(args ...string) ExecOption {
	return func(c *execConfig) { c.Args = args }
}

// WithEnv sets environment variables for the command.
func WithEnv(env map[string]string) ExecOption {
	return func(c *execConfig) { c.Env = env }
}

// WithCwd sets the working directory for the command.
func WithCwd(cwd string) ExecOption {
	return func(c *execConfig) { c.Cwd = cwd }
}

// WithTimeout sets the timeout in seconds for the command.
func WithTimeout(seconds int) ExecOption {
	return func(c *execConfig) { c.Timeout = seconds }
}

// ExecResult holds the result of a command execution.
type ExecResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

// FileInfo holds metadata about a file or directory.
type FileInfo struct {
	Name    string `json:"name" msgpack:"name"`
	Size    int64  `json:"size" msgpack:"size"`
	IsDir   bool   `json:"is_dir" msgpack:"is_dir"`
	ModTime int64  `json:"mod_time" msgpack:"mod_time"`
}

// FileEvent represents a file system change notification.
type FileEvent struct {
	Path string `json:"path" msgpack:"path"`
	Type string `json:"type" msgpack:"type"` // "created", "modified", "deleted"
}

// CancelFunc is returned by event-watching methods to stop listening.
type CancelFunc func()

// authRequest is the body sent to POST /api/v1/auth/token.
type authRequest struct {
	APIKey string `json:"api_key"`
}

// authResponse is the response from POST /api/v1/auth/token.
type authResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

// execRequest is the body sent to POST /api/v1/sandboxes/{id}/exec.
type execRequest struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}
