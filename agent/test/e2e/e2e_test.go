//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

var (
	agentURL = envOr("E2E_AGENT_URL", "http://localhost:8080")
	apiKey   = envOr("E2E_API_KEY", "e2e-test-key")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getToken(t *testing.T) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"api_key": apiKey})
	resp, err := http.Post(agentURL+"/api/v1/auth/token", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("get token: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("get token: status %d", resp.StatusCode)
	}
	var result struct {
		Token string `json:"token"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Token
}

func authReq(method, url string, body []byte, token string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

func TestE2E_HealthCheck(t *testing.T) {
	resp, err := http.Get(agentURL + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz: expected 200, got %d", resp.StatusCode)
	}
}

func TestE2E_SandboxLifecycle(t *testing.T) {
	token := getToken(t)

	// Create sandbox
	createBody, _ := json.Marshal(map[string]any{
		"template":        "base",
		"timeout_seconds": 300,
	})
	resp, err := authReq("POST", agentURL+"/api/v1/sandboxes", createBody, token)
	if err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Fatalf("create sandbox: expected 201, got %d", resp.StatusCode)
	}

	var sandbox struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	json.NewDecoder(resp.Body).Decode(&sandbox)
	if sandbox.ID == "" {
		t.Fatal("sandbox ID is empty")
	}
	t.Logf("created sandbox: %s (status: %s)", sandbox.ID, sandbox.Status)

	// Wait for sandbox to be running
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := authReq("GET", fmt.Sprintf("%s/api/v1/sandboxes/%s", agentURL, sandbox.ID), nil, token)
		if err != nil {
			t.Fatalf("get sandbox: %v", err)
		}
		json.NewDecoder(resp.Body).Decode(&sandbox)
		resp.Body.Close()
		if sandbox.Status == "running" {
			break
		}
		time.Sleep(2 * time.Second)
	}
	if sandbox.Status != "running" {
		t.Fatalf("sandbox not running after 60s, status: %s", sandbox.Status)
	}
	t.Logf("sandbox %s is running", sandbox.ID)

	// Execute command
	execBody, _ := json.Marshal(map[string]any{
		"command":         "echo",
		"args":            []string{"hello-e2e"},
		"timeout_seconds": 10,
	})
	resp, err = authReq("POST", fmt.Sprintf("%s/api/v1/sandboxes/%s/exec", agentURL, sandbox.ID), execBody, token)
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("exec: expected 200, got %d", resp.StatusCode)
	}

	var execResult struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
	}
	json.NewDecoder(resp.Body).Decode(&execResult)
	if execResult.ExitCode != 0 {
		t.Errorf("exec exit code: expected 0, got %d", execResult.ExitCode)
	}
	if execResult.Stdout == "" {
		t.Error("exec stdout is empty")
	}
	t.Logf("exec result: exit=%d stdout=%q", execResult.ExitCode, execResult.Stdout)

	// Delete sandbox
	resp, err = authReq("DELETE", fmt.Sprintf("%s/api/v1/sandboxes/%s", agentURL, sandbox.ID), nil, token)
	if err != nil {
		t.Fatalf("delete sandbox: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete sandbox: expected 204, got %d", resp.StatusCode)
	}
	t.Logf("sandbox %s deleted", sandbox.ID)

	// Verify deleted
	resp, err = authReq("GET", fmt.Sprintf("%s/api/v1/sandboxes/%s", agentURL, sandbox.ID), nil, token)
	if err != nil {
		t.Fatalf("get deleted sandbox: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("expected 404 for deleted sandbox, got %d", resp.StatusCode)
	}
}
