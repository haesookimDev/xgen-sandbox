package xgen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/xgen-sandbox/sdk-go/protocol"
	"github.com/xgen-sandbox/sdk-go/transport"
)

// Sandbox represents a running sandbox instance.
type Sandbox struct {
	ID   string
	Info SandboxInfo

	httpT  *transport.HTTPTransport
	ws     *transport.WSTransport
	wsMu   sync.Mutex
	status SandboxStatus
}

func newSandbox(info SandboxInfo, httpT *transport.HTTPTransport) *Sandbox {
	return &Sandbox{
		ID:     info.ID,
		Info:   info,
		httpT:  httpT,
		status: info.Status,
	}
}

// Status returns the current sandbox status.
func (s *Sandbox) Status() SandboxStatus {
	return s.status
}

// GetPreviewURL returns the preview URL for a specific port.
func (s *Sandbox) GetPreviewURL(port int) string {
	if s.Info.PreviewURLs == nil {
		return ""
	}
	return s.Info.PreviewURLs[port]
}

// ensureWS lazily connects the WebSocket transport.
func (s *Sandbox) ensureWS(ctx context.Context) (*transport.WSTransport, error) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()

	if s.ws != nil {
		return s.ws, nil
	}

	wsURL := s.httpT.WsURL(s.ID)
	token := s.httpT.Token()
	ws := transport.NewWSTransport(wsURL, token)
	if err := ws.Connect(ctx); err != nil {
		return nil, err
	}
	s.ws = ws
	return ws, nil
}

// Exec runs a command in the sandbox and returns the result.
// It uses the REST API endpoint.
func (s *Sandbox) Exec(ctx context.Context, command string, opts ...ExecOption) (*ExecResult, error) {
	cfg := &execConfig{Timeout: 30}
	for _, opt := range opts {
		opt(cfg)
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	args := append(parts[1:], cfg.Args...)

	body := execRequest{
		Command:        parts[0],
		Args:           args,
		Env:            cfg.Env,
		Cwd:            cfg.Cwd,
		TimeoutSeconds: cfg.Timeout,
	}

	path := fmt.Sprintf("/api/v1/sandboxes/%s/exec", s.ID)
	data, status, err := s.httpT.Do(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("exec failed: %d %s", status, string(data))
	}

	var result ExecResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("decode exec result: %w", err)
	}
	return &result, nil
}

// ReadFile reads a file from the sandbox and returns its raw bytes.
func (s *Sandbox) ReadFile(ctx context.Context, path string) ([]byte, error) {
	ws, err := s.ensureWS(ctx)
	if err != nil {
		return nil, err
	}

	payload, err := protocol.EncodePayload(map[string]string{"path": path})
	if err != nil {
		return nil, err
	}

	resp, err := ws.Request(ctx, protocol.FsRead, 0, payload)
	if err != nil {
		return nil, err
	}
	return resp.Payload, nil
}

// ReadTextFile reads a file from the sandbox and returns its content as a string.
func (s *Sandbox) ReadTextFile(ctx context.Context, path string) (string, error) {
	data, err := s.ReadFile(ctx, path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// WriteFile writes content to a file in the sandbox.
func (s *Sandbox) WriteFile(ctx context.Context, path string, content []byte) error {
	ws, err := s.ensureWS(ctx)
	if err != nil {
		return err
	}

	payload, err := protocol.EncodePayload(map[string]any{
		"path":    path,
		"content": content,
	})
	if err != nil {
		return err
	}

	_, err = ws.Request(ctx, protocol.FsWrite, 0, payload)
	return err
}

// ListDir lists files in a directory in the sandbox.
func (s *Sandbox) ListDir(ctx context.Context, path string) ([]FileInfo, error) {
	ws, err := s.ensureWS(ctx)
	if err != nil {
		return nil, err
	}

	payload, err := protocol.EncodePayload(map[string]string{"path": path})
	if err != nil {
		return nil, err
	}

	resp, err := ws.Request(ctx, protocol.FsList, 0, payload)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	if err := protocol.DecodePayload(resp.Payload, &files); err != nil {
		return nil, fmt.Errorf("decode file list: %w", err)
	}
	return files, nil
}

// RemoveFile removes a file or directory from the sandbox.
func (s *Sandbox) RemoveFile(ctx context.Context, path string, recursive bool) error {
	ws, err := s.ensureWS(ctx)
	if err != nil {
		return err
	}

	payload, err := protocol.EncodePayload(map[string]any{
		"path":      path,
		"recursive": recursive,
	})
	if err != nil {
		return err
	}

	_, err = ws.Request(ctx, protocol.FsRemove, 0, payload)
	return err
}

// WatchFiles watches a path for file changes. Returns a CancelFunc to stop watching.
func (s *Sandbox) WatchFiles(ctx context.Context, path string, callback func(FileEvent)) (CancelFunc, error) {
	ws, err := s.ensureWS(ctx)
	if err != nil {
		return nil, err
	}

	// Register event handler.
	cleanup := ws.On(protocol.FsEvent, func(env protocol.Envelope) {
		var event FileEvent
		if err := protocol.DecodePayload(env.Payload, &event); err == nil {
			callback(event)
		}
	})

	// Send watch request.
	payload, err := protocol.EncodePayload(map[string]string{"path": path})
	if err != nil {
		cleanup()
		return nil, err
	}

	err = ws.Send(ctx, protocol.Envelope{
		Type:    protocol.FsWatch,
		Channel: 0,
		ID:      0,
		Payload: payload,
	})
	if err != nil {
		cleanup()
		return nil, err
	}

	cancel := func() {
		cleanup()
		// Send unwatch request (best effort).
		unwatchPayload, err := protocol.EncodePayload(map[string]any{
			"path":    path,
			"unwatch": true,
		})
		if err == nil {
			_ = ws.Send(context.Background(), protocol.Envelope{
				Type:    protocol.FsWatch,
				Channel: 0,
				ID:      0,
				Payload: unwatchPayload,
			})
		}
	}
	return cancel, nil
}

// OnPortOpen registers a callback for port open events. Returns a CancelFunc to stop listening.
func (s *Sandbox) OnPortOpen(ctx context.Context, callback func(port int)) (CancelFunc, error) {
	ws, err := s.ensureWS(ctx)
	if err != nil {
		return nil, err
	}

	cleanup := ws.On(protocol.PortOpen, func(env protocol.Envelope) {
		var data struct {
			Port int `msgpack:"port"`
		}
		if err := protocol.DecodePayload(env.Payload, &data); err == nil {
			callback(data.Port)
		}
	})

	return CancelFunc(cleanup), nil
}

// KeepAlive sends a keep-alive signal for the sandbox.
func (s *Sandbox) KeepAlive(ctx context.Context) error {
	path := fmt.Sprintf("/api/v1/sandboxes/%s/keepalive", s.ID)
	_, status, err := s.httpT.Do(ctx, http.MethodPost, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("keepalive failed: %d", status)
	}
	return nil
}

// Destroy stops and deletes the sandbox.
func (s *Sandbox) Destroy(ctx context.Context) error {
	if s.ws != nil {
		_ = s.ws.Close()
		s.ws = nil
	}

	path := fmt.Sprintf("/api/v1/sandboxes/%s", s.ID)
	_, status, err := s.httpT.Do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("delete sandbox failed: %d", status)
	}
	s.status = StatusStopped
	return nil
}

// Close closes the WebSocket connection without deleting the sandbox.
func (s *Sandbox) Close() error {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()

	if s.ws != nil {
		err := s.ws.Close()
		s.ws = nil
		return err
	}
	return nil
}
