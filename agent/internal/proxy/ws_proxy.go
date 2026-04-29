package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/xgen-sandbox/agent/internal/sandbox"
	"github.com/xgen-sandbox/agent/pkg/protocol"
)

// WSProxy handles WebSocket proxying between clients and sandbox sidecars.
type WSProxy struct {
	sandboxMgr *sandbox.Manager
	sidecarURL func(podIP string) string
}

// NewWSProxy creates a new WebSocket proxy.
func NewWSProxy(sandboxMgr *sandbox.Manager) *WSProxy {
	return &WSProxy{
		sandboxMgr: sandboxMgr,
		sidecarURL: defaultSidecarURL,
	}
}

func defaultSidecarURL(podIP string) string {
	return fmt.Sprintf("ws://%s:9000/ws", podIP)
}

// HandleClientWS handles a client WebSocket connection and proxies to the sidecar.
// Each client gets its own dedicated sidecar connection to prevent cascading failures.
func (p *WSProxy) HandleClientWS(w http.ResponseWriter, r *http.Request, sandboxID string) {
	sbx, err := p.sandboxMgr.Get(sandboxID)
	if err != nil {
		http.Error(w, "sandbox not found", http.StatusNotFound)
		return
	}
	if sbx.PodIP == "" {
		http.Error(w, "sandbox not ready", http.StatusServiceUnavailable)
		return
	}

	log.Printf("ws proxy: client connecting for sandbox %s", sandboxID)
	clientConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("accept client ws: %v", err)
		return
	}
	defer clientConn.CloseNow()
	log.Printf("ws proxy: client connected for sandbox %s", sandboxID)

	// Create a dedicated sidecar connection for this client session.
	sidecarConn, _, err := websocket.Dial(r.Context(), p.sidecarURL(sbx.PodIP), nil)
	if err != nil {
		log.Printf("ws proxy: connect to sidecar failed for %s: %v", sandboxID, err)
		clientConn.Close(websocket.StatusInternalError, "failed to connect to sandbox")
		return
	}
	defer sidecarConn.CloseNow()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Bidirectional proxy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Sidecar
	go func() {
		defer wg.Done()
		defer cancel()
		proxyWS(ctx, clientConn, sidecarConn, "client->sidecar")
	}()

	// Sidecar -> Client
	go func() {
		defer wg.Done()
		defer cancel()
		proxyWS(ctx, sidecarConn, clientConn, "sidecar->client")
	}()

	wg.Wait()
}

func proxyWS(ctx context.Context, src, dst *websocket.Conn, direction string) {
	for {
		msgType, data, err := src.Read(ctx)
		if err != nil {
			log.Printf("ws proxy [%s]: read error: %v", direction, err)
			return
		}
		if len(data) >= 1 {
			log.Printf("ws proxy [%s]: forwarding msg type=0x%02x len=%d", direction, data[0], len(data))
		}
		if err := dst.Write(ctx, msgType, data); err != nil {
			log.Printf("ws proxy [%s]: write error: %v", direction, err)
			return
		}
	}
}

// ExecResult holds the result of a synchronous command execution.
type ExecResult struct {
	ExitCode         int
	Stdout           string
	Stderr           string
	Truncated        bool
	StdoutTruncated  bool
	StderrTruncated  bool
	TruncationMarker string
	ArtifactPath     *string
}

type ExecOptions struct {
	MaxOutputBytes int
	MaxStdoutBytes int
	MaxStderrBytes int
	ArtifactPath   string
}

const truncationMarker = "\n...[truncated]\n"

// ExecSync executes a command synchronously via a temporary sidecar WebSocket connection.
func (p *WSProxy) ExecSync(ctx context.Context, sandboxID string, command string, args []string, env map[string]string, cwd string, timeout time.Duration) (*ExecResult, error) {
	return p.ExecSyncWithOptions(ctx, sandboxID, command, args, env, cwd, timeout, ExecOptions{})
}

// ExecSyncWithOptions executes a command synchronously with LLM-safe output limits.
func (p *WSProxy) ExecSyncWithOptions(ctx context.Context, sandboxID string, command string, args []string, env map[string]string, cwd string, timeout time.Duration, opts ExecOptions) (*ExecResult, error) {
	sbx, err := p.sandboxMgr.Get(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox not found: %s", sandboxID)
	}
	if sbx.PodIP == "" {
		return nil, fmt.Errorf("sandbox not ready: %s", sandboxID)
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, _, err := websocket.Dial(execCtx, p.sidecarURL(sbx.PodIP), nil)
	if err != nil {
		return nil, fmt.Errorf("connect to sidecar: %w", err)
	}
	defer conn.CloseNow()

	// Read and discard the initial SandboxReady message.
	if _, _, err := conn.Read(execCtx); err != nil {
		return nil, fmt.Errorf("read ready message: %w", err)
	}

	// Build and send ExecStart.
	channelID := uint32(time.Now().UnixNano() & 0xFFFFFFFF)
	envelope, err := protocol.NewEnvelope(protocol.MsgExecStart, channelID, 0, protocol.ExecStartPayload{
		Command: command,
		Args:    args,
		Env:     env,
		Cwd:     cwd,
		TTY:     false,
	})
	if err != nil {
		return nil, fmt.Errorf("build exec envelope: %w", err)
	}
	wire, err := protocol.Encode(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode exec envelope: %w", err)
	}
	if err := conn.Write(execCtx, websocket.MessageBinary, wire); err != nil {
		return nil, fmt.Errorf("send exec start: %w", err)
	}

	// Read responses until ExecExit.
	result := &ExecResult{TruncationMarker: truncationMarker}
	capture := newExecCapture(opts)
	for {
		_, data, err := conn.Read(execCtx)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		msg, err := protocol.Decode(data)
		if err != nil {
			continue
		}

		switch msg.Type {
		case protocol.MsgExecStdout:
			capture.writeStdout(msg.Payload)
		case protocol.MsgExecStderr:
			capture.writeStderr(msg.Payload)
		case protocol.MsgExecExit:
			var exit protocol.ExecExitPayload
			if err := protocol.DecodePayload(msg.Payload, &exit); err == nil {
				result.ExitCode = exit.ExitCode
			}
			capture.apply(result)
			if result.Truncated {
				path, err := writeExecArtifact(execCtx, conn, channelID, opts.ArtifactPath, command, args, result, capture)
				if err == nil && path != "" {
					result.ArtifactPath = &path
				}
			}
			return result, nil
		case protocol.MsgAck:
			// Process started acknowledgement, continue.
		case protocol.MsgError:
			var errPayload protocol.ErrorPayload
			if err := protocol.DecodePayload(msg.Payload, &errPayload); err == nil {
				return nil, fmt.Errorf("exec error: %s: %s", errPayload.Code, errPayload.Message)
			}
			return nil, fmt.Errorf("exec error (could not decode payload)")
		}
	}
}

type execCapture struct {
	opts            ExecOptions
	stdout          []byte
	stderr          []byte
	artifactStdout  []byte
	artifactStderr  []byte
	stdoutTruncated bool
	stderrTruncated bool
}

func newExecCapture(opts ExecOptions) *execCapture {
	return &execCapture{opts: opts}
}

func (c *execCapture) writeStdout(data []byte) {
	c.artifactStdout = append(c.artifactStdout, data...)
	limit, limited := c.streamLimit(c.opts.MaxStdoutBytes)
	c.stdout, c.stdoutTruncated = appendLimited(c.stdout, data, limit, limited, c.stdoutTruncated)
}

func (c *execCapture) writeStderr(data []byte) {
	c.artifactStderr = append(c.artifactStderr, data...)
	limit, limited := c.streamLimit(c.opts.MaxStderrBytes)
	c.stderr, c.stderrTruncated = appendLimited(c.stderr, data, limit, limited, c.stderrTruncated)
}

func (c *execCapture) streamLimit(streamLimit int) (int, bool) {
	limit := streamLimit
	limited := limit > 0
	if c.opts.MaxOutputBytes > 0 {
		remaining := c.opts.MaxOutputBytes - len(c.stdout) - len(c.stderr)
		if remaining < 0 {
			remaining = 0
		}
		if !limited || remaining < limit {
			limit = remaining
		}
		limited = true
	}
	return limit, limited
}

func appendLimited(dst, data []byte, limit int, limited bool, alreadyTruncated bool) ([]byte, bool) {
	if !limited {
		return append(dst, data...), alreadyTruncated
	}
	if alreadyTruncated || len(dst) >= limit {
		return dst, true
	}
	remaining := limit - len(dst)
	if len(data) <= remaining {
		return append(dst, data...), false
	}
	dst = append(dst, data[:remaining]...)
	dst = append(dst, []byte(truncationMarker)...)
	return dst, true
}

func (c *execCapture) apply(result *ExecResult) {
	result.Stdout = string(c.stdout)
	result.Stderr = string(c.stderr)
	result.StdoutTruncated = c.stdoutTruncated
	result.StderrTruncated = c.stderrTruncated
	result.Truncated = c.stdoutTruncated || c.stderrTruncated
	if !result.Truncated {
		result.TruncationMarker = ""
	}
}

func writeExecArtifact(ctx context.Context, conn *websocket.Conn, channelID uint32, artifactPath, command string, args []string, result *ExecResult, capture *execCapture) (string, error) {
	if artifactPath == "" {
		artifactPath = fmt.Sprintf(".xgen/artifacts/exec-%d.json", time.Now().UnixNano())
	}
	content, err := json.MarshalIndent(map[string]any{
		"command":   command,
		"args":      args,
		"exit_code": result.ExitCode,
		"stdout":    string(capture.artifactStdout),
		"stderr":    string(capture.artifactStderr),
	}, "", "  ")
	if err != nil {
		return "", err
	}
	payload, err := protocol.EncodePayload(protocol.FsWritePayload{
		Path:    artifactPath,
		Content: content,
		Mode:    0644,
	})
	if err != nil {
		return "", err
	}
	env, err := protocol.NewEnvelope(protocol.MsgFsWrite, channelID, uint32(time.Now().UnixNano()), nil)
	if err != nil {
		return "", err
	}
	env.Payload = payload
	wire, err := protocol.Encode(env)
	if err != nil {
		return "", err
	}
	if err := conn.Write(ctx, websocket.MessageBinary, wire); err != nil {
		return "", err
	}
	return artifactPath, nil
}
