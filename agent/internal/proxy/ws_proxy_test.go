package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/xgen-sandbox/agent/api/v1"
	"github.com/xgen-sandbox/agent/internal/sandbox"
	"github.com/xgen-sandbox/agent/pkg/protocol"
	"nhooyr.io/websocket"
)

func TestExecSyncClosesTemporarySidecarConnection(t *testing.T) {
	var active int32
	closed := make(chan struct{}, 3)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept sidecar websocket: %v", err)
			return
		}
		atomic.AddInt32(&active, 1)
		defer func() {
			conn.CloseNow()
			atomic.AddInt32(&active, -1)
			closed <- struct{}{}
		}()

		ctx := r.Context()
		ready, err := protocol.NewEnvelope(protocol.MsgSandboxReady, 0, 0, nil)
		if err != nil {
			t.Errorf("build ready envelope: %v", err)
			return
		}
		readyWire, err := protocol.Encode(ready)
		if err != nil {
			t.Errorf("encode ready envelope: %v", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageBinary, readyWire); err != nil {
			t.Errorf("write ready envelope: %v", err)
			return
		}

		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("read exec start: %v", err)
			return
		}
		start, err := protocol.Decode(data)
		if err != nil {
			t.Errorf("decode exec start: %v", err)
			return
		}

		stdout := &protocol.Envelope{Type: protocol.MsgExecStdout, Channel: start.Channel, ID: start.ID, Payload: []byte("ok")}
		stdoutWire, err := protocol.Encode(stdout)
		if err != nil {
			t.Errorf("encode stdout: %v", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageBinary, stdoutWire); err != nil {
			t.Errorf("write stdout: %v", err)
			return
		}

		exit, err := protocol.NewEnvelope(protocol.MsgExecExit, start.Channel, start.ID, protocol.ExecExitPayload{ExitCode: 0})
		if err != nil {
			t.Errorf("build exit envelope: %v", err)
			return
		}
		exitWire, err := protocol.Encode(exit)
		if err != nil {
			t.Errorf("encode exit envelope: %v", err)
			return
		}
		if err := conn.Write(ctx, websocket.MessageBinary, exitWire); err != nil {
			t.Errorf("write exit: %v", err)
			return
		}
	}))
	defer server.Close()

	mgr := sandbox.NewManager()
	mgr.Recover("sbx-1", "base", "ignored", nil, false, nil, nil, nil, time.Now(), time.Now().Add(time.Hour), true)

	proxy := NewWSProxy(mgr)
	proxy.sidecarURL = func(string) string {
		return "ws" + strings.TrimPrefix(server.URL, "http")
	}

	for i := 0; i < 3; i++ {
		result, err := proxy.ExecSync(context.Background(), "sbx-1", "echo", []string{"ok"}, nil, "", time.Second)
		if err != nil {
			t.Fatalf("ExecSync() error: %v", err)
		}
		if result.ExitCode != 0 || result.Stdout != "ok" {
			t.Fatalf("ExecSync() = exit=%d stdout=%q", result.ExitCode, result.Stdout)
		}

		select {
		case <-closed:
		case <-time.After(time.Second):
			t.Fatal("temporary sidecar connection was not closed")
		}
		if got := atomic.LoadInt32(&active); got != 0 {
			t.Fatalf("active sidecar connections after ExecSync = %d, want 0", got)
		}
	}
}

func TestHandleClientWSUsesRequestScopedSidecarConnection(t *testing.T) {
	var active int32
	sidecarClosed := make(chan struct{}, 1)

	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept sidecar websocket: %v", err)
			return
		}
		atomic.AddInt32(&active, 1)
		defer func() {
			conn.CloseNow()
			atomic.AddInt32(&active, -1)
			sidecarClosed <- struct{}{}
		}()

		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		if err := conn.Write(r.Context(), websocket.MessageBinary, data); err != nil {
			t.Errorf("echo sidecar message: %v", err)
		}
	}))
	defer sidecar.Close()

	mgr := sandbox.NewManager()
	mgr.Recover("sbx-1", "base", "ignored", nil, false, nil, nil, nil, time.Now(), time.Now().Add(time.Hour), true)

	proxy := NewWSProxy(mgr)
	proxy.sidecarURL = func(string) string {
		return "ws" + strings.TrimPrefix(sidecar.URL, "http")
	}

	clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.HandleClientWS(w, r, "sbx-1")
	}))
	defer clientServer.Close()

	client, _, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(clientServer.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial client websocket: %v", err)
	}

	if err := client.Write(context.Background(), websocket.MessageBinary, []byte{protocol.MsgPing}); err != nil {
		t.Fatalf("write client message: %v", err)
	}
	_, got, err := client.Read(context.Background())
	if err != nil {
		t.Fatalf("read client echo: %v", err)
	}
	if string(got) != string([]byte{protocol.MsgPing}) {
		t.Fatalf("echoed message = %v, want ping byte", got)
	}
	client.CloseNow()

	select {
	case <-sidecarClosed:
	case <-time.After(time.Second):
		t.Fatal("request-scoped sidecar connection was not closed")
	}
	if got := atomic.LoadInt32(&active); got != 0 {
		t.Fatalf("active sidecar connections after client close = %d, want 0", got)
	}
}

func TestHandleClientWSRejectsNotReadySandboxWithoutDialingSidecar(t *testing.T) {
	var dials int32
	mgr := sandbox.NewManager()
	sbx := mgr.Create("base", time.Hour, nil, false, nil, nil, nil)
	mgr.SetStatus(sbx.ID, v1.StatusStarting)

	proxy := NewWSProxy(mgr)
	proxy.sidecarURL = func(string) string {
		atomic.AddInt32(&dials, 1)
		return "ws://127.0.0.1:1/ws"
	}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()
	proxy.HandleClientWS(rec, req, sbx.ID)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := atomic.LoadInt32(&dials); got != 0 {
		t.Fatalf("sidecar dials = %d, want 0", got)
	}
}

func TestExecCaptureTruncatesOutput(t *testing.T) {
	capture := newExecCapture(ExecOptions{MaxStdoutBytes: 5, MaxStderrBytes: 3})
	capture.writeStdout([]byte("hello world"))
	capture.writeStderr([]byte("abcdef"))

	result := &ExecResult{}
	capture.apply(result)

	if !result.Truncated || !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatalf("expected truncation flags, got %#v", result)
	}
	if result.Stdout != "hello"+truncationMarker {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if result.Stderr != "abc"+truncationMarker {
		t.Fatalf("stderr = %q", result.Stderr)
	}
}

func TestExecCaptureKeepsUnboundedOutput(t *testing.T) {
	capture := newExecCapture(ExecOptions{})
	capture.writeStdout([]byte("hello"))
	capture.writeStderr([]byte("world"))

	result := &ExecResult{}
	capture.apply(result)

	if result.Truncated || result.TruncationMarker != "" {
		t.Fatalf("unexpected truncation: %#v", result)
	}
	if result.Stdout != "hello" || result.Stderr != "world" {
		t.Fatalf("unexpected output: %#v", result)
	}
}

func TestExecCaptureMaxOutputBytesIsCombined(t *testing.T) {
	capture := newExecCapture(ExecOptions{MaxOutputBytes: 8})
	capture.writeStdout([]byte("hello"))
	capture.writeStderr([]byte("world"))

	result := &ExecResult{}
	capture.apply(result)

	if !result.Truncated || result.StdoutTruncated || !result.StderrTruncated {
		t.Fatalf("unexpected truncation flags: %#v", result)
	}
	if result.Stdout != "hello" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if result.Stderr != "wor"+truncationMarker {
		t.Fatalf("stderr = %q", result.Stderr)
	}
}
