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
	TimeoutMs      int64             `json:"timeout_ms,omitempty"`
	Resources      *Resources        `json:"resources,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Ports          []int             `json:"ports,omitempty"`
	GUI            bool              `json:"gui,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Capabilities   []string          `json:"capabilities,omitempty"`
}

// Resources specifies sandbox resource limits.
type Resources struct {
	CPU    string `json:"cpu,omitempty"`
	Memory string `json:"memory,omitempty"`
	Disk   string `json:"disk,omitempty"`
}

// SandboxInfo holds metadata about a sandbox returned by the API.
type SandboxInfo struct {
	ID           string            `json:"id"`
	Status       SandboxStatus     `json:"status"`
	Template     string            `json:"template"`
	WsURL        string            `json:"ws_url"`
	PreviewURLs  map[int]string    `json:"preview_urls"`
	VncURL       string            `json:"vnc_url,omitempty"`
	CreatedAt    string            `json:"created_at"`
	ExpiresAt    string            `json:"expires_at"`
	CreatedAtMs  int64             `json:"created_at_ms,omitempty"`
	ExpiresAtMs  int64             `json:"expires_at_ms,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	FromWarmPool bool              `json:"from_warm_pool,omitempty"`
}

type APIError struct {
	Status    int            `json:"-"`
	Code      string         `json:"code,omitempty"`
	Message   string         `json:"message,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

// ExecOption is a functional option for Exec.
type ExecOption func(*execConfig)

type execConfig struct {
	Args           []string
	Env            map[string]string
	Cwd            string
	Timeout        int // seconds
	MaxOutputBytes int
	MaxStdoutBytes int
	MaxStderrBytes int
	ArtifactPath   string
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

func WithMaxOutputBytes(n int) ExecOption {
	return func(c *execConfig) { c.MaxOutputBytes = n }
}

func WithMaxStdoutBytes(n int) ExecOption {
	return func(c *execConfig) { c.MaxStdoutBytes = n }
}

func WithMaxStderrBytes(n int) ExecOption {
	return func(c *execConfig) { c.MaxStderrBytes = n }
}

func WithExecArtifactPath(path string) ExecOption {
	return func(c *execConfig) { c.ArtifactPath = path }
}

// ExecResult holds the result of a command execution.
type ExecResult struct {
	ExitCode         int     `json:"exit_code"`
	Stdout           string  `json:"stdout"`
	Stderr           string  `json:"stderr"`
	Truncated        bool    `json:"truncated"`
	StdoutTruncated  bool    `json:"stdout_truncated,omitempty"`
	StderrTruncated  bool    `json:"stderr_truncated,omitempty"`
	TruncationMarker string  `json:"truncation_marker,omitempty"`
	ArtifactPath     *string `json:"artifact_path,omitempty"`
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

// ExecEvent represents a streaming execution event.
type ExecEvent struct {
	Type     string // "stdout", "stderr", "exit"
	Data     string
	ExitCode int
}

// TerminalOptions configures an interactive terminal session.
type TerminalOptions struct {
	Cols int
	Rows int
	Env  map[string]string
	Cwd  string
}

// CancelFunc is returned by event-watching methods to stop listening.
type CancelFunc func()

// authRequest is the body sent to POST /api/v1/auth/token.
type authRequest struct {
	APIKey string `json:"api_key"`
}

// authResponse is the response from POST /api/v1/auth/token.
type authResponse struct {
	Token       string `json:"token"`
	ExpiresAt   string `json:"expires_at"`
	ExpiresAtMs int64  `json:"expires_at_ms"`
}

// execRequest is the body sent to POST /api/v1/sandboxes/{id}/exec.
type execRequest struct {
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	TimeoutMs      int64             `json:"timeout_ms,omitempty"`
	MaxOutputBytes int               `json:"max_output_bytes,omitempty"`
	MaxStdoutBytes int               `json:"max_stdout_bytes,omitempty"`
	MaxStderrBytes int               `json:"max_stderr_bytes,omitempty"`
	ArtifactPath   string            `json:"artifact_path,omitempty"`
}
