package exec

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// Process represents a running process managed by the sidecar.
type Process struct {
	ID      uint32
	Cmd     *exec.Cmd
	ptmx    *os.File // nil if not a TTY session
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	done    chan struct{}
	exitErr error
	mu      sync.Mutex
}

// StartOptions configures process execution.
type StartOptions struct {
	Command string
	Args    []string
	Env     map[string]string
	Cwd     string
	TTY     bool
	Cols    uint16
	Rows    uint16
}

// Manager manages running processes.
type Manager struct {
	mu        sync.RWMutex
	processes map[uint32]*Process
	nextID    uint32
}

// NewManager creates a new process manager.
func NewManager() *Manager {
	return &Manager{
		processes: make(map[uint32]*Process),
		nextID:    1,
	}
}

// findRuntimePID finds the PID of the runtime container's init process ("sleep infinity").
func findRuntimePID() (string, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return "", fmt.Errorf("read /proc: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Skip non-numeric entries
		if e.Name()[0] < '1' || e.Name()[0] > '9' {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		// cmdline uses null bytes as separators: "sleep\x00infinity"
		if strings.Contains(string(cmdline), "sleep\x00infinity") {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("runtime container process not found")
}

// Start launches a new process and returns it.
// Commands are executed inside the runtime container's namespace via nsenter.
func (m *Manager) Start(opts StartOptions) (*Process, error) {
	runtimePID, err := findRuntimePID()
	if err != nil {
		return nil, fmt.Errorf("find runtime container: %w", err)
	}

	// Build nsenter command to enter the runtime container's mount and PID namespace
	cwd := opts.Cwd
	if cwd == "" {
		cwd = "/home/sandbox/workspace"
	}
	nsenterArgs := []string{
		"--target", runtimePID,
		"--mount",
		"--wd", cwd,
		"--",
	}
	nsenterArgs = append(nsenterArgs, opts.Command)
	nsenterArgs = append(nsenterArgs, opts.Args...)
	cmd := exec.Command("nsenter", nsenterArgs...)

	cmd.Env = os.Environ()
	for k, v := range opts.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set process group so we can kill the entire tree
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	m.mu.Lock()
	id := m.nextID
	m.nextID++
	m.mu.Unlock()

	proc := &Process{
		ID:   id,
		Cmd:  cmd,
		done: make(chan struct{}),
	}

	if opts.TTY {
		winSize := &pty.Winsize{
			Cols: opts.Cols,
			Rows: opts.Rows,
		}
		if winSize.Cols == 0 {
			winSize.Cols = 80
		}
		if winSize.Rows == 0 {
			winSize.Rows = 24
		}

		ptmx, err := pty.StartWithSize(cmd, winSize)
		if err != nil {
			return nil, fmt.Errorf("start pty: %w", err)
		}
		proc.ptmx = ptmx
	} else {
		var err error
		proc.stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("stdin pipe: %w", err)
		}

		// Use os.Pipe() instead of cmd.StdoutPipe()/StderrPipe() to prevent
		// cmd.Wait() from closing the read-ends before streaming goroutines
		// finish reading. See: https://pkg.go.dev/os/exec#Cmd.StdoutPipe
		stdoutR, stdoutW, err := os.Pipe()
		if err != nil {
			return nil, fmt.Errorf("stdout pipe: %w", err)
		}
		cmd.Stdout = stdoutW
		proc.stdout = stdoutR

		stderrR, stderrW, err := os.Pipe()
		if err != nil {
			stdoutR.Close()
			stdoutW.Close()
			return nil, fmt.Errorf("stderr pipe: %w", err)
		}
		cmd.Stderr = stderrW
		proc.stderr = stderrR

		if err := cmd.Start(); err != nil {
			stdoutR.Close()
			stdoutW.Close()
			stderrR.Close()
			stderrW.Close()
			return nil, fmt.Errorf("start process: %w", err)
		}
		// Close write ends in parent; child inherited them via fork.
		// When child exits, OS closes its copies → readers get EOF.
		stdoutW.Close()
		stderrW.Close()
	}

	// Wait for process exit in background
	go func() {
		proc.exitErr = cmd.Wait()
		close(proc.done)
	}()

	m.mu.Lock()
	m.processes[id] = proc
	m.mu.Unlock()

	return proc, nil
}

// Get returns a process by ID.
func (m *Manager) Get(id uint32) (*Process, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.processes[id]
	return p, ok
}

// Remove removes a process from the manager.
func (m *Manager) Remove(id uint32) {
	m.mu.Lock()
	delete(m.processes, id)
	m.mu.Unlock()
}

// KillAll sends SIGKILL to all running processes.
func (m *Manager) KillAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.processes {
		p.Kill()
	}
}

// Stdout returns a reader for stdout. For TTY sessions, this reads from the PTY.
func (p *Process) Stdout() io.Reader {
	if p.ptmx != nil {
		return p.ptmx
	}
	return p.stdout
}

// Stderr returns a reader for stderr. For TTY sessions, returns nil (stderr is merged into PTY).
func (p *Process) Stderr() io.Reader {
	if p.ptmx != nil {
		return nil
	}
	return p.stderr
}

// WriteStdin writes data to the process stdin or PTY.
func (p *Process) WriteStdin(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var w io.Writer
	if p.ptmx != nil {
		w = p.ptmx
	} else if p.stdin != nil {
		w = p.stdin
	} else {
		return fmt.Errorf("no stdin available")
	}
	_, err := w.Write(data)
	return err
}

// Resize changes the PTY window size.
func (p *Process) Resize(cols, rows uint16) error {
	if p.ptmx == nil {
		return fmt.Errorf("not a TTY session")
	}
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

// Signal sends a signal to the process.
func (p *Process) Signal(sig syscall.Signal) error {
	if p.Cmd.Process == nil {
		return fmt.Errorf("process not started")
	}
	// Signal the entire process group
	return syscall.Kill(-p.Cmd.Process.Pid, sig)
}

// Kill terminates the process.
func (p *Process) Kill() error {
	return p.Signal(syscall.SIGKILL)
}

// Done returns a channel that closes when the process exits.
func (p *Process) Done() <-chan struct{} {
	return p.done
}

// ExitCode returns the exit code of the process. Only valid after Done() is closed.
func (p *Process) ExitCode() int {
	if p.exitErr == nil {
		return 0
	}
	if exitErr, ok := p.exitErr.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// Close cleans up process resources.
func (p *Process) Close() {
	if p.ptmx != nil {
		p.ptmx.Close()
	}
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.stdout != nil {
		p.stdout.Close()
	}
	if p.stderr != nil {
		p.stderr.Close()
	}
}
