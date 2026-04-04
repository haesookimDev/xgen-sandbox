package xgen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/xgen-sandbox/sdk-go/transport"
)

// Client is the top-level entry point for the xgen-sandbox SDK.
type Client struct {
	http *transport.HTTPTransport
}

// NewClient creates a new xgen-sandbox client.
func NewClient(apiKey, agentURL string) *Client {
	return &Client{
		http: transport.NewHTTPTransport(agentURL, apiKey),
	}
}

// CreateSandbox creates a new sandbox and waits for it to reach "running" status.
func (c *Client) CreateSandbox(ctx context.Context, opts CreateSandboxOptions) (*Sandbox, error) {
	if opts.Template == "" {
		opts.Template = "base"
	}

	data, status, err := c.http.Do(ctx, http.MethodPost, "/api/v1/sandboxes", opts)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("create sandbox failed: %d %s", status, string(data))
	}

	var info SandboxInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("decode sandbox response: %w", err)
	}

	if info.Status != StatusRunning {
		if err := c.waitForRunning(ctx, info.ID, 60*time.Second); err != nil {
			return nil, err
		}
		updated, err := c.GetSandbox(ctx, info.ID)
		if err != nil {
			return nil, err
		}
		return updated, nil
	}

	return newSandbox(info, c.http), nil
}

// GetSandbox retrieves an existing sandbox by ID.
func (c *Client) GetSandbox(ctx context.Context, id string) (*Sandbox, error) {
	data, status, err := c.http.Do(ctx, http.MethodGet, "/api/v1/sandboxes/"+id, nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get sandbox failed: %d %s", status, string(data))
	}

	var info SandboxInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("decode sandbox response: %w", err)
	}
	return newSandbox(info, c.http), nil
}

// ListSandboxes returns all sandboxes.
func (c *Client) ListSandboxes(ctx context.Context) ([]SandboxInfo, error) {
	data, status, err := c.http.Do(ctx, http.MethodGet, "/api/v1/sandboxes", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list sandboxes failed: %d %s", status, string(data))
	}

	var infos []SandboxInfo
	if err := json.Unmarshal(data, &infos); err != nil {
		return nil, fmt.Errorf("decode sandbox list: %w", err)
	}
	return infos, nil
}

func (c *Client) waitForRunning(ctx context.Context, id string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, status, err := c.http.Do(ctx, http.MethodGet, "/api/v1/sandboxes/"+id, nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("get sandbox failed while waiting: %d", status)
		}

		var info SandboxInfo
		if err := json.Unmarshal(data, &info); err != nil {
			return err
		}

		switch info.Status {
		case StatusRunning:
			return nil
		case StatusError, StatusStopped:
			return fmt.Errorf("sandbox %s entered %s state", id, info.Status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
	return fmt.Errorf("sandbox %s did not become ready within %v", id, timeout)
}
