package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPTransport handles REST API calls and token management.
type HTTPTransport struct {
	baseURL  string
	apiKey   string
	client   *http.Client
	mu       sync.Mutex
	token    string
	expireAt time.Time
}

// NewHTTPTransport creates a new HTTP transport.
func NewHTTPTransport(agentURL, apiKey string) *HTTPTransport {
	return &HTTPTransport{
		baseURL: strings.TrimRight(agentURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Token returns the current auth token, or empty string if not yet obtained.
func (h *HTTPTransport) Token() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.token
}

// WsURL returns the WebSocket URL for a sandbox.
func (h *HTTPTransport) WsURL(sandboxID string) string {
	wsBase := strings.Replace(h.baseURL, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	return fmt.Sprintf("%s/api/v1/sandboxes/%s/ws", wsBase, sandboxID)
}

// ensureToken fetches or refreshes the auth token as needed.
func (h *HTTPTransport) ensureToken(ctx context.Context) (string, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.token != "" && time.Now().Before(h.expireAt.Add(-60*time.Second)) {
		return h.token, nil
	}

	body, err := json.Marshal(map[string]string{"api_key": h.apiKey})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/api/v1/auth/token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("auth failed: %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("auth decode failed: %w", err)
	}

	h.token = result.Token
	if t, err := time.Parse(time.RFC3339, result.ExpiresAt); err == nil {
		h.expireAt = t
	}
	return h.token, nil
}

// Do performs an authenticated HTTP request and returns the response body.
func (h *HTTPTransport) Do(ctx context.Context, method, path string, reqBody any) ([]byte, int, error) {
	token, err := h.ensureToken(ctx)
	if err != nil {
		return nil, 0, err
	}

	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, h.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}
