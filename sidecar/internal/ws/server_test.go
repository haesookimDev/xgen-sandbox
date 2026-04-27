package ws

import (
	"testing"

	execpkg "github.com/xgen-sandbox/sidecar/internal/exec"
	fspkg "github.com/xgen-sandbox/sidecar/internal/fs"
)

func TestResolveProcessID_ChannelFallback(t *testing.T) {
	s := NewServer(execpkg.NewManager(), fspkg.NewHandler(t.TempDir()))
	s.registerChannelProcess(42, 7)

	got, ok := s.resolveProcessID(0, 42)
	if !ok {
		t.Fatal("expected channel fallback to resolve")
	}
	if got != 7 {
		t.Fatalf("expected process 7, got %d", got)
	}
}

func TestResolveProcessID_ExplicitProcessIDWins(t *testing.T) {
	s := NewServer(execpkg.NewManager(), fspkg.NewHandler(t.TempDir()))
	s.registerChannelProcess(42, 7)

	got, ok := s.resolveProcessID(9, 42)
	if !ok {
		t.Fatal("expected explicit process id to resolve")
	}
	if got != 9 {
		t.Fatalf("expected process 9, got %d", got)
	}
}

func TestResolveProcessID_Unregister(t *testing.T) {
	s := NewServer(execpkg.NewManager(), fspkg.NewHandler(t.TempDir()))
	s.registerChannelProcess(42, 7)
	s.unregisterChannelProcess(42)

	if _, ok := s.resolveProcessID(0, 42); ok {
		t.Fatal("expected channel mapping to be removed")
	}
}
