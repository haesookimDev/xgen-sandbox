package proxy

import (
	"context"
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
	mu         sync.RWMutex
	conns      map[string]*websocket.Conn // sandboxID -> sidecar connection
}

// NewWSProxy creates a new WebSocket proxy.
func NewWSProxy(sandboxMgr *sandbox.Manager) *WSProxy {
	return &WSProxy{
		sandboxMgr: sandboxMgr,
		conns:      make(map[string]*websocket.Conn),
	}
}

// ConnectToSidecar establishes a WebSocket connection to a sandbox's sidecar.
func (p *WSProxy) ConnectToSidecar(ctx context.Context, sandboxID, podIP string) error {
	sidecarURL := fmt.Sprintf("ws://%s:9000/ws", podIP)
	conn, _, err := websocket.Dial(ctx, sidecarURL, nil)
	if err != nil {
		return fmt.Errorf("connect to sidecar: %w", err)
	}

	p.mu.Lock()
	p.conns[sandboxID] = conn
	p.mu.Unlock()

	return nil
}

// DisconnectSidecar closes the connection to a sandbox's sidecar.
func (p *WSProxy) DisconnectSidecar(sandboxID string) {
	p.mu.Lock()
	conn, ok := p.conns[sandboxID]
	if ok {
		conn.Close(websocket.StatusNormalClosure, "sandbox destroyed")
		delete(p.conns, sandboxID)
	}
	p.mu.Unlock()
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
	sidecarURL := fmt.Sprintf("ws://%s:9000/ws", sbx.PodIP)
	sidecarConn, _, err := websocket.Dial(r.Context(), sidecarURL, nil)
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

// SendToSidecar sends a raw message to a sidecar.
func (p *WSProxy) SendToSidecar(ctx context.Context, sandboxID string, data []byte) error {
	p.mu.RLock()
	conn, ok := p.conns[sandboxID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("no sidecar connection for sandbox: %s", sandboxID)
	}
	return conn.Write(ctx, websocket.MessageBinary, data)
}

// ReadFromSidecar reads a message from a sidecar. Blocking.
func (p *WSProxy) ReadFromSidecar(ctx context.Context, sandboxID string) ([]byte, error) {
	p.mu.RLock()
	conn, ok := p.conns[sandboxID]
	p.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no sidecar connection for sandbox: %s", sandboxID)
	}
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ExecResult holds the result of a synchronous command execution.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ExecSync executes a command synchronously via a temporary sidecar WebSocket connection.
func (p *WSProxy) ExecSync(ctx context.Context, sandboxID string, command string, args []string, env map[string]string, cwd string, timeout time.Duration) (*ExecResult, error) {
	sbx, err := p.sandboxMgr.Get(sandboxID)
	if err != nil {
		return nil, fmt.Errorf("sandbox not found: %s", sandboxID)
	}
	if sbx.PodIP == "" {
		return nil, fmt.Errorf("sandbox not ready: %s", sandboxID)
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	sidecarURL := fmt.Sprintf("ws://%s:9000/ws", sbx.PodIP)
	conn, _, err := websocket.Dial(execCtx, sidecarURL, nil)
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
	result := &ExecResult{}
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
			result.Stdout += string(msg.Payload)
		case protocol.MsgExecStderr:
			result.Stderr += string(msg.Payload)
		case protocol.MsgExecExit:
			var exit protocol.ExecExitPayload
			if err := protocol.DecodePayload(msg.Payload, &exit); err == nil {
				result.ExitCode = exit.ExitCode
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
