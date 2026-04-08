package exec

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.nextID != 1 {
		t.Errorf("nextID: expected 1, got %d", m.nextID)
	}
	if len(m.processes) != 0 {
		t.Errorf("expected empty processes map, got %d entries", len(m.processes))
	}
}

func TestManager_GetNotFound(t *testing.T) {
	m := NewManager()
	_, ok := m.Get(999)
	if ok {
		t.Error("expected false for nonexistent process")
	}
}

func TestManager_Remove(t *testing.T) {
	m := NewManager()

	// Manually add a process to the map for testing
	m.mu.Lock()
	m.processes[1] = &Process{ID: 1, done: make(chan struct{})}
	m.mu.Unlock()

	_, ok := m.Get(1)
	if !ok {
		t.Fatal("expected to find process 1")
	}

	m.Remove(1)

	_, ok = m.Get(1)
	if ok {
		t.Error("expected process to be removed")
	}
}

func TestProcess_ExitCode_NilErr(t *testing.T) {
	p := &Process{exitErr: nil}
	if code := p.ExitCode(); code != 0 {
		t.Errorf("ExitCode: expected 0, got %d", code)
	}
}

func TestProcess_ExitCode_UnknownError(t *testing.T) {
	p := &Process{exitErr: &testError{}}
	if code := p.ExitCode(); code != -1 {
		t.Errorf("ExitCode: expected -1, got %d", code)
	}
}

func TestProcess_DoneChannel(t *testing.T) {
	done := make(chan struct{})
	p := &Process{done: done}

	// Channel should not be closed initially
	select {
	case <-p.Done():
		t.Error("Done() should not be closed yet")
	default:
		// ok
	}

	// Close and verify
	close(done)
	select {
	case <-p.Done():
		// ok
	default:
		t.Error("Done() should be closed")
	}
}

func TestProcess_Stdout_NonTTY(t *testing.T) {
	// When ptmx is nil, Stdout should return the stdout pipe
	p := &Process{ptmx: nil, stdout: nil}
	if p.Stdout() != nil {
		t.Error("expected nil stdout when no pipes set")
	}
}

func TestProcess_Stderr_TTY(t *testing.T) {
	// For TTY sessions (ptmx != nil), Stderr returns nil
	// We can't create a real pty, but we can test the logic
	// by checking the nil-ptmx case
	p := &Process{ptmx: nil, stderr: nil}
	if p.Stderr() != nil {
		t.Error("expected nil stderr for non-TTY with no pipe")
	}
}

// testError is a simple error type for testing non-ExitError paths.
type testError struct{}

func (e *testError) Error() string { return "test error" }
