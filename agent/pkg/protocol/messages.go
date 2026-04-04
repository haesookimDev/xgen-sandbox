package protocol

// Message types for the WebSocket protocol.
const (
	// Control
	MsgPing         uint8 = 0x01
	MsgPong         uint8 = 0x02
	MsgError        uint8 = 0x03
	MsgAck          uint8 = 0x04

	// Channel management
	MsgChannelOpen  uint8 = 0x10
	MsgChannelClose uint8 = 0x11
	MsgChannelData  uint8 = 0x12

	// Execution
	MsgExecStart    uint8 = 0x20
	MsgExecStdin    uint8 = 0x21
	MsgExecStdout   uint8 = 0x22
	MsgExecStderr   uint8 = 0x23
	MsgExecExit     uint8 = 0x24
	MsgExecSignal   uint8 = 0x25
	MsgExecResize   uint8 = 0x26

	// Filesystem
	MsgFsRead       uint8 = 0x30
	MsgFsWrite      uint8 = 0x31
	MsgFsList       uint8 = 0x32
	MsgFsRemove     uint8 = 0x33
	MsgFsWatch      uint8 = 0x34
	MsgFsEvent      uint8 = 0x35

	// Network/Port
	MsgPortOpen     uint8 = 0x40
	MsgPortClose    uint8 = 0x41

	// Sandbox lifecycle
	MsgSandboxReady uint8 = 0x50
	MsgSandboxError uint8 = 0x51
	MsgSandboxStats uint8 = 0x52
)

// Envelope is the wire format for all WebSocket messages.
type Envelope struct {
	Type    uint8  `msgpack:"type"`
	Channel uint32 `msgpack:"chan"`
	ID      uint32 `msgpack:"id"`
	Payload []byte `msgpack:"payload"`
}

// ExecStartPayload is sent by the client to start a command.
type ExecStartPayload struct {
	Command string            `msgpack:"command" json:"command"`
	Args    []string          `msgpack:"args" json:"args"`
	Env     map[string]string `msgpack:"env,omitempty" json:"env,omitempty"`
	Cwd     string            `msgpack:"cwd,omitempty" json:"cwd,omitempty"`
	TTY     bool              `msgpack:"tty" json:"tty"`
	Cols    uint16            `msgpack:"cols,omitempty" json:"cols,omitempty"`
	Rows    uint16            `msgpack:"rows,omitempty" json:"rows,omitempty"`
}

// ExecExitPayload is sent when a process exits.
type ExecExitPayload struct {
	ExitCode int `msgpack:"exit_code" json:"exit_code"`
}

// ExecResizePayload is sent to resize a TTY.
type ExecResizePayload struct {
	Cols uint16 `msgpack:"cols" json:"cols"`
	Rows uint16 `msgpack:"rows" json:"rows"`
}

// ExecSignalPayload is sent to signal a running process.
type ExecSignalPayload struct {
	Signal string `msgpack:"signal" json:"signal"`
}

// ErrorPayload is sent when an error occurs.
type ErrorPayload struct {
	Code    string `msgpack:"code" json:"code"`
	Message string `msgpack:"message" json:"message"`
}

// FsReadPayload requests reading a file.
type FsReadPayload struct {
	Path string `msgpack:"path" json:"path"`
}

// FsWritePayload requests writing to a file.
type FsWritePayload struct {
	Path    string `msgpack:"path" json:"path"`
	Content []byte `msgpack:"content" json:"content"`
	Mode    uint32 `msgpack:"mode,omitempty" json:"mode,omitempty"`
}

// FsListPayload requests listing a directory.
type FsListPayload struct {
	Path string `msgpack:"path" json:"path"`
}

// FileInfo describes a file entry.
type FileInfo struct {
	Name    string `msgpack:"name" json:"name"`
	Size    int64  `msgpack:"size" json:"size"`
	IsDir   bool   `msgpack:"is_dir" json:"is_dir"`
	ModTime int64  `msgpack:"mod_time" json:"mod_time"`
}

// FsRemovePayload requests removing a file or directory.
type FsRemovePayload struct {
	Path      string `msgpack:"path" json:"path"`
	Recursive bool   `msgpack:"recursive" json:"recursive"`
}

// FsWatchPayload requests watching a path for changes.
type FsWatchPayload struct {
	Path    string `msgpack:"path" json:"path"`
	Unwatch bool   `msgpack:"unwatch,omitempty" json:"unwatch,omitempty"`
}

// FsEventPayload is sent when a watched file changes.
type FsEventPayload struct {
	Path string `msgpack:"path" json:"path"`
	Type string `msgpack:"type" json:"type"` // created, modified, deleted
}

// PortOpenPayload is sent when a port starts listening.
type PortOpenPayload struct {
	Port uint16 `msgpack:"port" json:"port"`
}

// PortClosePayload is sent when a port stops listening.
type PortClosePayload struct {
	Port uint16 `msgpack:"port" json:"port"`
}

// SandboxStatsPayload reports resource usage.
type SandboxStatsPayload struct {
	CPUPercent    float64 `msgpack:"cpu_percent" json:"cpu_percent"`
	MemoryBytes   uint64  `msgpack:"memory_bytes" json:"memory_bytes"`
	DiskUsedBytes uint64  `msgpack:"disk_used_bytes" json:"disk_used_bytes"`
}
