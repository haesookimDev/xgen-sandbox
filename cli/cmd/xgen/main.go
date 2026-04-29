package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/vmihailenco/msgpack/v5"
	"nhooyr.io/websocket"
)

const (
	msgError        byte = 0x03
	msgAck          byte = 0x04
	msgExecStart    byte = 0x20
	msgExecStdout   byte = 0x22
	msgExecStderr   byte = 0x23
	msgExecExit     byte = 0x24
	msgFsRead       byte = 0x30
	msgFsWrite      byte = 0x31
	msgFsList       byte = 0x32
	msgFsRemove     byte = 0x33
	msgPortOpen     byte = 0x40
	msgSandboxReady byte = 0x50
	headerSize           = 9

	defaultSessionIdleTTLMS        int64 = 30 * 60 * 1000
	defaultSessionKeepaliveAfterMS int64 = 5 * 60 * 1000
)

type cliConfig struct {
	agentURL   string
	apiKey     string
	token      string
	apiVersion string
}

type apiClient struct {
	cfg    cliConfig
	http   *http.Client
	token  string
	expire time.Time
}

type apiError struct {
	Status    int            `json:"status,omitempty"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
	SandboxID string         `json:"sandbox_id,omitempty"`
	CommandID string         `json:"command_id,omitempty"`
}

func (e *apiError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}

type envelope struct {
	Type    byte
	Channel uint32
	ID      uint32
	Payload []byte
}

type sandboxInfo struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	Template     string            `json:"template"`
	WsURL        string            `json:"ws_url"`
	PreviewURLs  map[int]string    `json:"preview_urls,omitempty"`
	VncURL       *string           `json:"vnc_url,omitempty"`
	CreatedAtMs  int64             `json:"created_at_ms,omitempty"`
	ExpiresAtMs  int64             `json:"expires_at_ms,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	FromWarmPool bool              `json:"from_warm_pool,omitempty"`
}

type sessionRecord struct {
	SessionID    string            `json:"session_id"`
	SandboxID    string            `json:"sandbox_id"`
	Template     string            `json:"template,omitempty"`
	Cwd          string            `json:"cwd,omitempty"`
	Ports        []int             `json:"ports,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	CreatedAtMs  int64             `json:"created_at_ms,omitempty"`
	ExpiresAtMs  int64             `json:"expires_at_ms,omitempty"`
	LastUsedAtMs int64             `json:"last_used_at_ms"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type sessionPolicy struct {
	IdleTTLMS        int64 `json:"idle_ttl_ms"`
	KeepaliveAfterMS int64 `json:"keepalive_after_ms"`
}

type gcResult struct {
	Removed   []sessionRecord `json:"removed"`
	Destroyed []sessionRecord `json:"destroyed"`
	Kept      []sessionRecord `json:"kept"`
	Errors    []apiError      `json:"errors,omitempty"`
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error {
	*s = append(*s, v)
	return nil
}

type intList []int

func (i *intList) String() string { return fmt.Sprint([]int(*i)) }
func (i *intList) Set(v string) error {
	n, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	*i = append(*i, n)
	return nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cfg, rest, err := parseGlobal(args)
	if err != nil {
		return fail(err)
	}
	if len(rest) == 0 {
		return fail(errors.New("missing command"))
	}
	client := &apiClient{cfg: cfg, http: &http.Client{Timeout: 30 * time.Second}, token: cfg.token}

	switch rest[0] {
	case "auth":
		return runAuth(client, rest[1:])
	case "create":
		return runCreate(client, rest[1:])
	case "exec":
		return runExec(client, rest[1:])
	case "fs":
		return runFS(client, rest[1:])
	case "port":
		return runPort(client, rest[1:])
	case "session":
		return runSession(client, rest[1:])
	default:
		return fail(fmt.Errorf("unknown command: %s", rest[0]))
	}
}

func parseGlobal(args []string) (cliConfig, []string, error) {
	fs := flag.NewFlagSet("xgen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := cliConfig{
		agentURL:   envOr("XGEN_AGENT_URL", "http://localhost:8080"),
		apiKey:     os.Getenv("XGEN_API_KEY"),
		token:      os.Getenv("XGEN_TOKEN"),
		apiVersion: envOr("XGEN_API_VERSION", "v2"),
	}
	fs.StringVar(&cfg.agentURL, "agent-url", cfg.agentURL, "xgen agent URL")
	fs.StringVar(&cfg.apiKey, "api-key", cfg.apiKey, "API key")
	fs.StringVar(&cfg.token, "token", cfg.token, "bearer token")
	fs.StringVar(&cfg.apiVersion, "api-version", cfg.apiVersion, "API version: v1 or v2")
	if err := fs.Parse(args); err != nil {
		return cfg, nil, err
	}
	if cfg.apiVersion != "v1" && cfg.apiVersion != "v2" {
		return cfg, nil, fmt.Errorf("api-version must be v1 or v2")
	}
	cfg.agentURL = strings.TrimRight(cfg.agentURL, "/")
	return cfg, fs.Args(), nil
}

func runAuth(client *apiClient, args []string) int {
	if len(args) == 0 || args[0] != "token" {
		return fail(errors.New("usage: xgen auth token --json"))
	}
	fs := flag.NewFlagSet("auth token", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	_ = fs.Bool("json", true, "emit JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return fail(err)
	}
	var out map[string]any
	if err := client.postRaw(context.Background(), "/auth/token", map[string]string{"api_key": client.cfg.apiKey}, &out, false); err != nil {
		return fail(err)
	}
	return printJSON(out)
}

func runCreate(client *apiClient, args []string) int {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	template := fs.String("template", "base", "runtime template")
	ttlMs := fs.Int64("ttl-ms", 0, "sandbox TTL in milliseconds")
	gui := fs.Bool("gui", false, "enable GUI")
	_ = fs.Bool("json", true, "emit JSON")
	var metadata stringList
	var ports intList
	var caps stringList
	fs.Var(&metadata, "metadata", "metadata key=value")
	fs.Var(&ports, "port", "exposed port")
	fs.Var(&caps, "capability", "runtime capability")
	if err := fs.Parse(args); err != nil {
		return fail(err)
	}
	body := map[string]any{"template": *template}
	if *ttlMs > 0 {
		body["timeout_ms"] = *ttlMs
	}
	if *gui {
		body["gui"] = true
	}
	if len(ports) > 0 {
		body["ports"] = []int(ports)
	}
	if len(caps) > 0 {
		body["capabilities"] = []string(caps)
	}
	meta, err := parseKV(metadata)
	if err != nil {
		return fail(err)
	}
	sessionID := "sess_" + randomHex(8)
	meta["xgen_session_id"] = sessionID
	meta["xgen_session_registry"] = "cli"
	if cwd, err := os.Getwd(); err == nil {
		meta["xgen_cwd"] = cwd
	}
	body["metadata"] = meta

	var info sandboxInfo
	if err := client.post(context.Background(), "/sandboxes", body, &info); err != nil {
		return fail(err)
	}
	rec := sessionFromSandbox(sessionID, info)
	_ = upsertSession(rec)
	return printJSON(map[string]any{"sandbox": info, "session": rec})
}

func runExec(client *apiClient, args []string) int {
	if len(args) == 0 {
		return fail(errors.New("usage: xgen exec <sandbox-id> --json --timeout-ms 30000 -- <cmd>"))
	}
	sandboxID := args[0]
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	stream := fs.Bool("stream", false, "stream output")
	_ = fs.Bool("json", true, "emit JSON")
	_ = fs.Bool("jsonl", false, "emit JSON lines")
	timeoutMs := fs.Int64("timeout-ms", 30000, "timeout in milliseconds")
	maxOutput := fs.Int("max-output-bytes", 0, "truncate combined output to this many bytes")
	if err := fs.Parse(args[1:]); err != nil {
		return fail(err)
	}
	cmd := fs.Args()
	if len(cmd) == 0 {
		return fail(errors.New("missing command after --"))
	}
	if err := autoKeepaliveSession(context.Background(), client, sandboxID); err != nil {
		return fail(withSandbox(err, sandboxID))
	}
	if *stream {
		code, err := execStream(client, sandboxID, cmd, *timeoutMs)
		if err != nil {
			return fail(withSandbox(err, sandboxID))
		}
		touchSession(sandboxID)
		return code
	}

	body := map[string]any{"command": cmd[0]}
	if len(cmd) > 1 {
		body["args"] = cmd[1:]
	}
	if *timeoutMs > 0 {
		body["timeout_ms"] = *timeoutMs
	}
	var out struct {
		ExitCode int    `json:"exit_code"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	if err := client.post(context.Background(), "/sandboxes/"+url.PathEscape(sandboxID)+"/exec", body, &out); err != nil {
		return fail(withSandbox(err, sandboxID))
	}
	truncated := false
	if *maxOutput > 0 {
		out.Stdout, out.Stderr, truncated = truncatePair(out.Stdout, out.Stderr, *maxOutput)
	}
	touchSession(sandboxID)
	return printJSON(map[string]any{
		"sandbox_id": sandboxID,
		"exit_code":  out.ExitCode,
		"stdout":     out.Stdout,
		"stderr":     out.Stderr,
		"truncated":  truncated,
	})
}

func runFS(client *apiClient, args []string) int {
	if len(args) < 1 {
		return fail(errors.New("usage: xgen fs read|write|list|rm ..."))
	}
	switch args[0] {
	case "read":
		if len(args) < 3 {
			return fail(errors.New("usage: xgen fs read <sandbox-id> <path> --json"))
		}
		if err := autoKeepaliveSession(context.Background(), client, args[1]); err != nil {
			return fail(withSandbox(err, args[1]))
		}
		data, err := wsRequestPayload(client, args[1], msgFsRead, map[string]any{"path": args[2]})
		if err != nil {
			return fail(withSandbox(err, args[1]))
		}
		out := map[string]any{"sandbox_id": args[1], "path": args[2], "content_base64": base64.StdEncoding.EncodeToString(data), "size": len(data)}
		if utf8.Valid(data) {
			out["content_text"] = string(data)
		}
		touchSession(args[1])
		return printJSON(out)
	case "write":
		if len(args) < 4 {
			return fail(errors.New("usage: xgen fs write <sandbox-id> <path> <content> --json"))
		}
		if err := autoKeepaliveSession(context.Background(), client, args[1]); err != nil {
			return fail(withSandbox(err, args[1]))
		}
		if _, err := wsRequestPayload(client, args[1], msgFsWrite, map[string]any{"path": args[2], "content": []byte(args[3])}); err != nil {
			return fail(withSandbox(err, args[1]))
		}
		touchSession(args[1])
		return printJSON(map[string]any{"sandbox_id": args[1], "path": args[2], "written": true})
	case "list":
		if len(args) < 3 {
			return fail(errors.New("usage: xgen fs list <sandbox-id> <path> --json"))
		}
		if err := autoKeepaliveSession(context.Background(), client, args[1]); err != nil {
			return fail(withSandbox(err, args[1]))
		}
		data, err := wsRequestPayload(client, args[1], msgFsList, map[string]any{"path": args[2]})
		if err != nil {
			return fail(withSandbox(err, args[1]))
		}
		var entries []map[string]any
		if err := msgpack.Unmarshal(data, &entries); err != nil {
			return fail(err)
		}
		touchSession(args[1])
		return printJSON(map[string]any{"sandbox_id": args[1], "path": args[2], "entries": entries})
	case "rm":
		if len(args) < 3 {
			return fail(errors.New("usage: xgen fs rm <sandbox-id> <path> --json"))
		}
		fs := flag.NewFlagSet("fs rm", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		recursive := fs.Bool("recursive", false, "remove recursively")
		_ = fs.Bool("json", true, "emit JSON")
		if err := fs.Parse(args[3:]); err != nil {
			return fail(err)
		}
		if err := autoKeepaliveSession(context.Background(), client, args[1]); err != nil {
			return fail(withSandbox(err, args[1]))
		}
		if _, err := wsRequestPayload(client, args[1], msgFsRemove, map[string]any{"path": args[2], "recursive": *recursive}); err != nil {
			return fail(withSandbox(err, args[1]))
		}
		touchSession(args[1])
		return printJSON(map[string]any{"sandbox_id": args[1], "path": args[2], "removed": true})
	default:
		return fail(fmt.Errorf("unknown fs command: %s", args[0]))
	}
}

func runPort(client *apiClient, args []string) int {
	if len(args) < 1 || args[0] != "wait" || len(args) < 3 {
		return fail(errors.New("usage: xgen port wait <sandbox-id> <port> --timeout-ms 30000 --json"))
	}
	port, err := strconv.Atoi(args[2])
	if err != nil {
		return fail(err)
	}
	fs := flag.NewFlagSet("port wait", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	timeoutMs := fs.Int64("timeout-ms", 30000, "timeout in milliseconds")
	_ = fs.Bool("json", true, "emit JSON")
	if err := fs.Parse(args[3:]); err != nil {
		return fail(err)
	}
	if err := autoKeepaliveSession(context.Background(), client, args[1]); err != nil {
		return fail(withSandbox(err, args[1]))
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutMs)*time.Millisecond)
	defer cancel()
	if err := waitPort(ctx, client, args[1], uint16(port)); err != nil {
		return fail(withSandbox(err, args[1]))
	}
	touchSession(args[1])
	return printJSON(map[string]any{"sandbox_id": args[1], "port": port, "open": true})
}

func runSession(client *apiClient, args []string) int {
	if len(args) == 0 {
		return fail(errors.New("usage: xgen session list|get|keepalive|destroy|gc --json"))
	}
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("session list", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		_ = fs.Bool("json", true, "emit JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return fail(err)
		}
		sessions, err := loadSessions()
		if err != nil {
			return fail(err)
		}
		return printJSON(map[string]any{"sessions": sessions, "policy": currentSessionPolicy()})
	case "get":
		if len(args) < 2 {
			return fail(errors.New("usage: xgen session get <session-id> --json"))
		}
		fs := flag.NewFlagSet("session get", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		_ = fs.Bool("json", true, "emit JSON")
		if err := fs.Parse(args[2:]); err != nil {
			return fail(err)
		}
		rec, ok, err := getSession(args[1])
		if err != nil {
			return fail(err)
		}
		if !ok {
			return fail(&apiError{Code: "SESSION_NOT_FOUND", Message: "session not found", Retryable: false})
		}
		return printJSON(rec)
	case "keepalive":
		if len(args) < 2 {
			return fail(errors.New("usage: xgen session keepalive <session-id> --json"))
		}
		rec, ok, err := getSession(args[1])
		if err != nil {
			return fail(err)
		}
		if !ok {
			return fail(&apiError{Code: "SESSION_NOT_FOUND", Message: "session not found"})
		}
		if err := client.post(context.Background(), "/sandboxes/"+url.PathEscape(rec.SandboxID)+"/keepalive", nil, nil); err != nil {
			return fail(withSandbox(err, rec.SandboxID))
		}
		updated, _ := refreshSessionFromAPI(context.Background(), client, rec)
		return printJSON(map[string]any{"session_id": updated.SessionID, "sandbox_id": updated.SandboxID, "kept_alive": true, "session": updated})
	case "destroy":
		if len(args) < 2 {
			return fail(errors.New("usage: xgen session destroy <session-id> --json"))
		}
		rec, ok, err := getSession(args[1])
		if err != nil {
			return fail(err)
		}
		if !ok {
			return fail(&apiError{Code: "SESSION_NOT_FOUND", Message: "session not found"})
		}
		if err := client.delete(context.Background(), "/sandboxes/"+url.PathEscape(rec.SandboxID)); err != nil {
			return fail(withSandbox(err, rec.SandboxID))
		}
		_ = deleteSession(rec.SessionID)
		return printJSON(map[string]any{"session_id": rec.SessionID, "sandbox_id": rec.SandboxID, "destroyed": true})
	case "gc":
		fs := flag.NewFlagSet("session gc", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		destroy := fs.Bool("destroy", true, "destroy expired/idle tracked sandboxes")
		_ = fs.Bool("json", true, "emit JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return fail(err)
		}
		result, err := gcSessions(context.Background(), client, currentSessionPolicy(), *destroy)
		if err != nil {
			return fail(err)
		}
		return printJSON(result)
	default:
		return fail(fmt.Errorf("unknown session command: %s", args[0]))
	}
}

func (c *apiClient) apiPath(suffix string) string {
	return "/api/" + c.cfg.apiVersion + suffix
}

func (c *apiClient) ensureToken(ctx context.Context) (string, error) {
	if c.token != "" && (c.expire.IsZero() || time.Now().Before(c.expire.Add(-time.Minute))) {
		return c.token, nil
	}
	if c.cfg.apiKey == "" {
		return "", &apiError{Code: "UNAUTHORIZED", Message: "set XGEN_API_KEY or pass --api-key", Retryable: false}
	}
	var out struct {
		Token       string `json:"token"`
		ExpiresAt   string `json:"expires_at"`
		ExpiresAtMs int64  `json:"expires_at_ms"`
	}
	if err := c.postRaw(ctx, "/auth/token", map[string]string{"api_key": c.cfg.apiKey}, &out, false); err != nil {
		return "", err
	}
	c.token = out.Token
	if out.ExpiresAtMs > 0 {
		c.expire = time.UnixMilli(out.ExpiresAtMs)
	} else if out.ExpiresAt != "" {
		c.expire, _ = time.Parse(time.RFC3339, out.ExpiresAt)
	}
	return c.token, nil
}

func (c *apiClient) post(ctx context.Context, suffix string, body any, out any) error {
	return c.postRaw(ctx, suffix, body, out, true)
}

func (c *apiClient) postRaw(ctx context.Context, suffix string, body any, out any, auth bool) error {
	return c.do(ctx, http.MethodPost, suffix, body, out, auth)
}

func (c *apiClient) get(ctx context.Context, suffix string, out any) error {
	return c.do(ctx, http.MethodGet, suffix, nil, out, true)
}

func (c *apiClient) delete(ctx context.Context, suffix string) error {
	return c.do(ctx, http.MethodDelete, suffix, nil, nil, true)
}

func (c *apiClient) do(ctx context.Context, method, suffix string, body any, out any, auth bool) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.agentURL+c.apiPath(suffix), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if auth {
		token, err := c.ensureToken(ctx)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func decodeAPIError(status int, data []byte) error {
	var e apiError
	if err := json.Unmarshal(data, &e); err == nil && (e.Code != "" || e.Message != "") {
		e.Status = status
		if e.Message == "" {
			e.Message = http.StatusText(status)
		}
		return &e
	}
	return &apiError{Status: status, Code: "HTTP_ERROR", Message: string(data), Retryable: status >= 500}
}

func execStream(client *apiClient, sandboxID string, cmd []string, timeoutMs int64) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()
	conn, err := connectWS(ctx, client, sandboxID)
	if err != nil {
		return 1, err
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	channel := uint32(time.Now().UnixNano())
	payload, _ := msgpack.Marshal(map[string]any{"command": cmd[0], "args": cmd[1:], "tty": false})
	if err := writeEnvelope(ctx, conn, envelope{Type: msgExecStart, Channel: channel, Payload: payload}); err != nil {
		return 1, err
	}
	for {
		env, err := readEnvelope(ctx, conn)
		if err != nil {
			return 1, err
		}
		if env.Channel != channel && env.Type != msgError {
			continue
		}
		switch env.Type {
		case msgExecStdout:
			printJSONLine(map[string]any{"type": "stdout", "sandbox_id": sandboxID, "data": string(env.Payload)})
		case msgExecStderr:
			printJSONLine(map[string]any{"type": "stderr", "sandbox_id": sandboxID, "data": string(env.Payload)})
		case msgExecExit:
			var exit map[string]int
			_ = msgpack.Unmarshal(env.Payload, &exit)
			code := exit["exit_code"]
			printJSONLine(map[string]any{"type": "exit", "sandbox_id": sandboxID, "exit_code": code})
			return code, nil
		case msgError:
			return 1, decodeSidecarError(env.Payload)
		}
	}
}

func wsRequestPayload(client *apiClient, sandboxID string, msgType byte, payload any) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := connectWS(ctx, client, sandboxID)
	if err != nil {
		return nil, err
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	raw, err := msgpack.Marshal(payload)
	if err != nil {
		return nil, err
	}
	id := uint32(time.Now().UnixNano())
	if err := writeEnvelope(ctx, conn, envelope{Type: msgType, ID: id, Payload: raw}); err != nil {
		return nil, err
	}
	for {
		env, err := readEnvelope(ctx, conn)
		if err != nil {
			return nil, err
		}
		if env.ID != id {
			continue
		}
		if env.Type == msgError {
			return nil, decodeSidecarError(env.Payload)
		}
		return env.Payload, nil
	}
}

func waitPort(ctx context.Context, client *apiClient, sandboxID string, port uint16) error {
	conn, err := connectWS(ctx, client, sandboxID)
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")
	for {
		env, err := readEnvelope(ctx, conn)
		if err != nil {
			return err
		}
		if env.Type != msgPortOpen {
			continue
		}
		var data struct {
			Port uint16 `msgpack:"port"`
		}
		if msgpack.Unmarshal(env.Payload, &data) == nil && data.Port == port {
			return nil
		}
	}
}

func connectWS(ctx context.Context, client *apiClient, sandboxID string) (*websocket.Conn, error) {
	token, err := client.ensureToken(ctx)
	if err != nil {
		return nil, err
	}
	wsBase := strings.Replace(client.cfg.agentURL, "https://", "wss://", 1)
	wsBase = strings.Replace(wsBase, "http://", "ws://", 1)
	u, _ := url.Parse(wsBase + client.apiPath("/sandboxes/"+url.PathEscape(sandboxID)+"/ws"))
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	conn, _, err := websocket.Dial(ctx, u.String(), nil)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(32 * 1024 * 1024)
	for {
		env, err := readEnvelope(ctx, conn)
		if err != nil {
			conn.CloseNow()
			return nil, err
		}
		if env.Type == msgSandboxReady {
			return conn, nil
		}
	}
}

func writeEnvelope(ctx context.Context, conn *websocket.Conn, env envelope) error {
	buf := make([]byte, headerSize+len(env.Payload))
	buf[0] = env.Type
	binary.BigEndian.PutUint32(buf[1:5], env.Channel)
	binary.BigEndian.PutUint32(buf[5:9], env.ID)
	copy(buf[headerSize:], env.Payload)
	return conn.Write(ctx, websocket.MessageBinary, buf)
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (envelope, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return envelope{}, err
	}
	if len(data) < headerSize {
		return envelope{}, errors.New("short websocket frame")
	}
	return envelope{
		Type:    data[0],
		Channel: binary.BigEndian.Uint32(data[1:5]),
		ID:      binary.BigEndian.Uint32(data[5:9]),
		Payload: data[headerSize:],
	}, nil
}

func decodeSidecarError(payload []byte) error {
	var e struct {
		Code    string `msgpack:"code"`
		Message string `msgpack:"message"`
	}
	if msgpack.Unmarshal(payload, &e) != nil {
		return &apiError{Code: "SIDECAR_ERROR", Message: "sidecar error"}
	}
	return &apiError{Code: strings.ToUpper(e.Code), Message: e.Message}
}

func registryPath() (string, error) {
	if p := os.Getenv("XGEN_SESSION_REGISTRY"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".xgen", "sessions.json"), nil
}

func loadSessions() ([]sessionRecord, error) {
	path, err := registryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []sessionRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []sessionRecord
	if len(data) == 0 {
		return []sessionRecord{}, nil
	}
	return records, json.Unmarshal(data, &records)
}

func saveSessions(records []sessionRecord) error {
	path, err := registryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func upsertSession(rec sessionRecord) error {
	records, err := loadSessions()
	if err != nil {
		return err
	}
	for i := range records {
		if records[i].SandboxID == rec.SandboxID || records[i].SessionID == rec.SessionID {
			records[i] = rec
			return saveSessions(records)
		}
	}
	return saveSessions(append(records, rec))
}

func getSession(id string) (sessionRecord, bool, error) {
	records, err := loadSessions()
	if err != nil {
		return sessionRecord{}, false, err
	}
	for _, rec := range records {
		if rec.SessionID == id || rec.SandboxID == id {
			return rec, true, nil
		}
	}
	return sessionRecord{}, false, nil
}

func deleteSession(id string) error {
	records, err := loadSessions()
	if err != nil {
		return err
	}
	out := records[:0]
	for _, rec := range records {
		if rec.SessionID != id && rec.SandboxID != id {
			out = append(out, rec)
		}
	}
	return saveSessions(out)
}

func touchSession(sandboxID string) {
	records, err := loadSessions()
	if err != nil {
		return
	}
	now := time.Now().UnixMilli()
	changed := false
	for i := range records {
		if records[i].SandboxID == sandboxID {
			records[i].LastUsedAtMs = now
			changed = true
		}
	}
	if changed {
		_ = saveSessions(records)
	}
}

func autoKeepaliveSession(ctx context.Context, client *apiClient, sandboxID string) error {
	rec, ok, err := getSession(sandboxID)
	if err != nil || !ok {
		return err
	}
	policy := currentSessionPolicy()
	now := time.Now().UnixMilli()
	if sessionIdleExpired(rec, policy, now) {
		return &apiError{Code: "SESSION_IDLE_EXPIRED", Message: "session exceeded idle TTL; run xgen session gc or create a new sandbox", SandboxID: rec.SandboxID}
	}
	if sessionNeedsKeepalive(rec, policy, now) {
		if err := client.post(ctx, "/sandboxes/"+url.PathEscape(rec.SandboxID)+"/keepalive", nil, nil); err != nil {
			return err
		}
		_, err = refreshSessionFromAPI(ctx, client, rec)
		return err
	}
	touchSession(sandboxID)
	return nil
}

func refreshSessionFromAPI(ctx context.Context, client *apiClient, rec sessionRecord) (sessionRecord, error) {
	var info sandboxInfo
	if err := client.get(ctx, "/sandboxes/"+url.PathEscape(rec.SandboxID), &info); err != nil {
		return rec, err
	}
	rec.Template = info.Template
	rec.Ports = keys(info.PreviewURLs)
	rec.Capabilities = info.Capabilities
	rec.CreatedAtMs = info.CreatedAtMs
	rec.ExpiresAtMs = info.ExpiresAtMs
	rec.LastUsedAtMs = time.Now().UnixMilli()
	if info.Metadata != nil {
		rec.Metadata = info.Metadata
		if cwd := info.Metadata["xgen_cwd"]; cwd != "" {
			rec.Cwd = cwd
		}
	}
	_ = upsertSession(rec)
	return rec, nil
}

func currentSessionPolicy() sessionPolicy {
	return sessionPolicy{
		IdleTTLMS:        envInt64("XGEN_SESSION_IDLE_TTL_MS", defaultSessionIdleTTLMS),
		KeepaliveAfterMS: envInt64("XGEN_SESSION_KEEPALIVE_AFTER_MS", defaultSessionKeepaliveAfterMS),
	}
}

func sessionIdleExpired(rec sessionRecord, policy sessionPolicy, now int64) bool {
	return policy.IdleTTLMS > 0 && rec.LastUsedAtMs > 0 && now-rec.LastUsedAtMs > policy.IdleTTLMS
}

func sessionNeedsKeepalive(rec sessionRecord, policy sessionPolicy, now int64) bool {
	return rec.ExpiresAtMs > 0 && policy.KeepaliveAfterMS >= 0 && rec.ExpiresAtMs-now <= policy.KeepaliveAfterMS
}

func gcSessions(ctx context.Context, client *apiClient, policy sessionPolicy, destroy bool) (gcResult, error) {
	records, err := loadSessions()
	if err != nil {
		return gcResult{}, err
	}
	now := time.Now().UnixMilli()
	result := gcResult{}
	var kept []sessionRecord
	for _, rec := range records {
		expired := rec.ExpiresAtMs > 0 && rec.ExpiresAtMs <= now
		idle := sessionIdleExpired(rec, policy, now)
		if expired || idle {
			if destroy {
				if err := client.delete(ctx, "/sandboxes/"+url.PathEscape(rec.SandboxID)); err != nil && !isSandboxNotFound(err) {
					result.Errors = append(result.Errors, errorPayload(err, rec.SandboxID))
					kept = append(kept, rec)
					result.Kept = append(result.Kept, rec)
					continue
				} else {
					result.Destroyed = append(result.Destroyed, rec)
				}
			}
			result.Removed = append(result.Removed, rec)
			continue
		}
		updated, err := refreshSessionFromAPI(ctx, client, rec)
		if err != nil {
			if isSandboxNotFound(err) {
				result.Removed = append(result.Removed, rec)
				continue
			}
			result.Errors = append(result.Errors, errorPayload(err, rec.SandboxID))
			kept = append(kept, rec)
			continue
		}
		kept = append(kept, updated)
		result.Kept = append(result.Kept, updated)
	}
	if err := saveSessions(kept); err != nil {
		return result, err
	}
	return result, nil
}

func sessionFromSandbox(sessionID string, info sandboxInfo) sessionRecord {
	rec := sessionRecord{
		SessionID:    sessionID,
		SandboxID:    info.ID,
		Template:     info.Template,
		Ports:        keys(info.PreviewURLs),
		Capabilities: info.Capabilities,
		CreatedAtMs:  info.CreatedAtMs,
		ExpiresAtMs:  info.ExpiresAtMs,
		LastUsedAtMs: time.Now().UnixMilli(),
		Metadata:     info.Metadata,
	}
	if info.Metadata != nil {
		rec.Cwd = info.Metadata["xgen_cwd"]
	}
	return rec
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(buf)
}

func keys(m map[int]string) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func parseKV(vals []string) (map[string]string, error) {
	out := map[string]string{}
	for _, v := range vals {
		k, val, ok := strings.Cut(v, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid key=value: %s", v)
		}
		out[k] = val
	}
	return out, nil
}

func truncatePair(stdout, stderr string, limit int) (string, string, bool) {
	if len(stdout)+len(stderr) <= limit {
		return stdout, stderr, false
	}
	remaining := limit
	if len(stdout) > remaining {
		return stdout[:remaining], "", true
	}
	remaining -= len(stdout)
	if len(stderr) > remaining {
		stderr = stderr[:remaining]
	}
	return stdout, stderr, true
}

func withSandbox(err error, sandboxID string) error {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		apiErr.SandboxID = sandboxID
	}
	return err
}

func isSandboxNotFound(err error) bool {
	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Status == http.StatusNotFound || apiErr.Code == "SANDBOX_NOT_FOUND"
}

func errorPayload(err error, sandboxID string) apiError {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		out := *apiErr
		if out.SandboxID == "" {
			out.SandboxID = sandboxID
		}
		return out
	}
	return apiError{Code: "ERROR", Message: err.Error(), SandboxID: sandboxID}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func fail(err error) int {
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.Code == "" {
			apiErr.Code = "ERROR"
		}
		if apiErr.Message == "" {
			apiErr.Message = err.Error()
		}
		_ = json.NewEncoder(os.Stderr).Encode(apiErr)
		return 1
	}
	_ = json.NewEncoder(os.Stderr).Encode(apiError{Code: "ERROR", Message: err.Error(), Retryable: false})
	return 1
}

func printJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fail(err)
	}
	return 0
}

func printJSONLine(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
