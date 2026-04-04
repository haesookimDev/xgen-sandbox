package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"

	"nhooyr.io/websocket"

	"github.com/xgen-sandbox/agent/internal/sandbox"
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

	clientConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("accept client ws: %v", err)
		return
	}
	defer clientConn.CloseNow()

	// Get or create sidecar connection
	p.mu.RLock()
	sidecarConn, ok := p.conns[sandboxID]
	p.mu.RUnlock()

	if !ok {
		// Try to connect
		if err := p.ConnectToSidecar(r.Context(), sandboxID, sbx.PodIP); err != nil {
			clientConn.Close(websocket.StatusInternalError, "failed to connect to sandbox")
			return
		}
		p.mu.RLock()
		sidecarConn = p.conns[sandboxID]
		p.mu.RUnlock()
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Bidirectional proxy
	var wg sync.WaitGroup
	wg.Add(2)

	// Client -> Sidecar
	go func() {
		defer wg.Done()
		defer cancel()
		proxyWS(ctx, clientConn, sidecarConn)
	}()

	// Sidecar -> Client
	go func() {
		defer wg.Done()
		defer cancel()
		proxyWS(ctx, sidecarConn, clientConn)
	}()

	wg.Wait()
}

func proxyWS(ctx context.Context, src, dst *websocket.Conn) {
	for {
		msgType, data, err := src.Read(ctx)
		if err != nil {
			return
		}
		if err := dst.Write(ctx, msgType, data); err != nil {
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
