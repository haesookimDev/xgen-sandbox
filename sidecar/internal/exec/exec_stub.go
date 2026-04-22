//go:build !linux

// Package exec's real implementation requires Linux syscalls (chroot,
// setuid-via-SysProcAttr.Credential). This stub exists so the sidecar
// package imports cleanly on non-Linux hosts for `go vet` and IDE tooling.
// The sidecar binary itself only ships for Linux.
package exec

import (
	"fmt"
	"io"
	"sync"
	"syscall"
)

type Process struct {
	ID      uint32
	done    chan struct{}
	exitErr error
}

type StartOptions struct {
	Command string
	Args    []string
	Env     map[string]string
	Cwd     string
	TTY     bool
	Cols    uint16
	Rows    uint16
}

type Manager struct {
	mu        sync.RWMutex
	processes map[uint32]*Process
	nextID    uint32
}

func NewManager() *Manager {
	return &Manager{processes: make(map[uint32]*Process), nextID: 1}
}

func (m *Manager) Start(opts StartOptions) (*Process, error) {
	return nil, fmt.Errorf("exec is only supported on linux")
}

func (m *Manager) Get(id uint32) (*Process, bool) { return nil, false }
func (m *Manager) Remove(id uint32)               {}
func (m *Manager) KillAll()                       {}

func (p *Process) Stdout() io.Reader              { return nil }
func (p *Process) Stderr() io.Reader              { return nil }
func (p *Process) WriteStdin(data []byte) error   { return fmt.Errorf("unsupported") }
func (p *Process) Resize(cols, rows uint16) error { return fmt.Errorf("unsupported") }
func (p *Process) Signal(sig syscall.Signal) error {
	return fmt.Errorf("unsupported")
}
func (p *Process) Kill() error             { return fmt.Errorf("unsupported") }
func (p *Process) Done() <-chan struct{}   { return p.done }
func (p *Process) ExitCode() int           { return -1 }
func (p *Process) Close()                  {}
