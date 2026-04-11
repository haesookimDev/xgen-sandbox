# SDK Guide

xgen-sandbox provides official SDKs for TypeScript, Python, Go, and Rust. All SDKs share the same API pattern:

1. Create a client with API key and agent URL
2. Create a sandbox (optionally with template, ports, GUI)
3. Execute commands, manage files, stream output
4. Destroy the sandbox when done

---

## TypeScript

### Installation

```bash
npm install @xgen-sandbox/sdk
```

### Quick Start

```typescript
import { XgenClient } from "@xgen-sandbox/sdk";

const client = new XgenClient({
  apiKey: "xgen_dev_key",
  agentUrl: "http://localhost:8080",
});

const sandbox = await client.createSandbox({
  template: "nodejs",
  ports: [3000],
});

// Execute a command
const result = await sandbox.exec("echo", { args: ["hello"] });
console.log(result.stdout); // "hello\n"
console.log(result.exitCode); // 0

// File operations
await sandbox.writeFile("index.js", "console.log('hi')");
const content = await sandbox.readTextFile("index.js");
const files = await sandbox.listDir(".");

// Stream execution output
for await (const event of sandbox.execStream("node index.js")) {
  if (event.type === "stdout") process.stdout.write(event.data);
  if (event.type === "exit") console.log(`Exit: ${event.exitCode}`);
}

// Interactive terminal
const terminal = await sandbox.openTerminal({ cols: 120, rows: 40 });
terminal.onData((data) => process.stdout.write(data));
terminal.write("ls -la\n");
// ...
terminal.close();

// Watch for file changes
const watcher = sandbox.watchFiles("/home/sandbox/workspace", (event) => {
  console.log(`${event.type}: ${event.path}`);
});
// watcher.dispose() to stop

// Watch for port opens
const portWatcher = sandbox.onPortOpen((port) => {
  console.log(`Port ${port} opened: ${sandbox.getPreviewUrl(port)}`);
});

// Cleanup
await sandbox.destroy();
```

### Client Methods

```typescript
class XgenClient {
  constructor(options: { agentUrl: string; apiKey: string })
  createSandbox(options?: CreateSandboxOptions): Promise<Sandbox>
  getSandbox(id: string): Promise<Sandbox>
  listSandboxes(): Promise<SandboxInfo[]>
}
```

### Sandbox Methods

```typescript
class Sandbox {
  readonly id: string
  readonly info: SandboxInfo
  get status(): SandboxStatus
  get previewUrls(): Record<number, string>
  getPreviewUrl(port: number): string | undefined

  // Execution
  exec(command: string, options?: ExecOptions): Promise<ExecResult>
  execStream(command: string, options?: ExecOptions): AsyncIterable<ExecEvent>
  openTerminal(options?: TerminalOptions): Promise<Terminal>

  // Filesystem
  readFile(path: string): Promise<Uint8Array>
  readTextFile(path: string): Promise<string>
  writeFile(path: string, content: Uint8Array | string): Promise<void>
  listDir(path: string): Promise<FileInfo[]>
  removeFile(path: string, recursive?: boolean): Promise<void>

  // Events
  watchFiles(path: string, callback: (event: FileEvent) => void): Disposable
  onPortOpen(callback: (port: number) => void): Disposable

  // Lifecycle
  keepAlive(): Promise<void>
  destroy(): Promise<void>
}
```

---

## Python

### Installation

```bash
pip install xgen-sandbox
```

### Quick Start

```python
from xgen_sandbox import XgenClient

async with XgenClient("xgen_dev_key", "http://localhost:8080") as client:
    sandbox = await client.create_sandbox(template="python", ports=[8000])

    # Execute a command
    result = await sandbox.exec("python3", args=["-c", "print('hello')"])
    print(result.stdout)  # "hello\n"

    # File operations
    await sandbox.write_file("app.py", "print('hi')")
    content = await sandbox.read_text_file("app.py")
    files = await sandbox.list_dir(".")

    # Watch files
    watcher = sandbox.watch_files("/home/sandbox/workspace", lambda e: print(e))

    # Port events
    port_watcher = sandbox.on_port_open(lambda port: print(f"Port {port} opened"))

    # Cleanup
    await sandbox.destroy()
```

### Client Methods

```python
class XgenClient:
    def __init__(self, api_key: str, agent_url: str) -> None

    async def create_sandbox(
        self,
        template: str = "base",
        timeout_seconds: int | None = None,
        env: dict[str, str] | None = None,
        ports: list[int] | None = None,
        gui: bool | None = None,
        metadata: dict[str, str] | None = None,
    ) -> Sandbox

    async def get_sandbox(self, sandbox_id: str) -> Sandbox
    async def list_sandboxes(self) -> list[SandboxInfo]
    async def close(self) -> None
```

### Sandbox Methods

```python
class Sandbox:
    @property
    def id(self) -> str
    @property
    def status(self) -> SandboxStatus
    @property
    def preview_urls(self) -> dict[int, str]
    def get_preview_url(self, port: int) -> str | None

    async def exec(self, command: str, args=None, env=None, cwd=None, timeout=None) -> ExecResult
    async def read_file(self, path: str) -> bytes
    async def read_text_file(self, path: str) -> str
    async def write_file(self, path: str, content: bytes | str) -> None
    async def list_dir(self, path: str) -> list[FileInfo]
    async def remove_file(self, path: str, recursive: bool = False) -> None
    def watch_files(self, path: str, callback) -> Disposable
    def on_port_open(self, callback) -> Disposable
    async def keep_alive(self) -> None
    async def destroy(self) -> None
```

**Dependencies:** `httpx`, `websockets`, `msgpack`

---

## Go

### Installation

```bash
go get github.com/xgen-sandbox/sdk-go
```

### Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    xgen "github.com/xgen-sandbox/sdk-go"
)

func main() {
    ctx := context.Background()
    client := xgen.NewClient("xgen_dev_key", "http://localhost:8080")

    sandbox, err := client.CreateSandbox(ctx, xgen.CreateSandboxOptions{
        Template: "go",
        Ports:    []int{8080},
    })
    if err != nil {
        log.Fatal(err)
    }
    defer sandbox.Destroy(ctx)

    // Execute a command
    result, err := sandbox.Exec(ctx, "go", xgen.WithArgs("version"))
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(result.Stdout) // "go version go1.22..."

    // File operations
    sandbox.WriteFile(ctx, "main.go", []byte(`package main; func main() { println("hi") }`))
    content, _ := sandbox.ReadTextFile(ctx, "main.go")
    files, _ := sandbox.ListDir(ctx, ".")

    // Watch files
    cancel, _ := sandbox.WatchFiles(ctx, ".", func(event xgen.FileEvent) {
        fmt.Printf("%s: %s\n", event.Type, event.Path)
    })
    defer cancel()

    // Port events
    portCancel, _ := sandbox.OnPortOpen(ctx, func(port int) {
        fmt.Printf("Port %d opened: %s\n", port, sandbox.GetPreviewURL(port))
    })
    defer portCancel()
}
```

### Client Methods

```go
func NewClient(apiKey, agentURL string) *Client
func (c *Client) CreateSandbox(ctx context.Context, opts CreateSandboxOptions) (*Sandbox, error)
func (c *Client) GetSandbox(ctx context.Context, id string) (*Sandbox, error)
func (c *Client) ListSandboxes(ctx context.Context) ([]SandboxInfo, error)
```

### Sandbox Methods

```go
func (s *Sandbox) Status() SandboxStatus
func (s *Sandbox) GetPreviewURL(port int) string
func (s *Sandbox) Exec(ctx context.Context, command string, opts ...ExecOption) (*ExecResult, error)
func (s *Sandbox) ReadFile(ctx context.Context, path string) ([]byte, error)
func (s *Sandbox) ReadTextFile(ctx context.Context, path string) (string, error)
func (s *Sandbox) WriteFile(ctx context.Context, path string, content []byte) error
func (s *Sandbox) ListDir(ctx context.Context, path string) ([]FileInfo, error)
func (s *Sandbox) RemoveFile(ctx context.Context, path string, recursive bool) error
func (s *Sandbox) WatchFiles(ctx context.Context, path string, callback func(FileEvent)) (CancelFunc, error)
func (s *Sandbox) OnPortOpen(ctx context.Context, callback func(port int)) (CancelFunc, error)
func (s *Sandbox) KeepAlive(ctx context.Context) error
func (s *Sandbox) Destroy(ctx context.Context) error
func (s *Sandbox) Close() error
```

**Exec Options:** `WithArgs(...)`, `WithEnv(...)`, `WithCwd(...)`, `WithTimeout(...)`

**Dependencies:** `nhooyr.io/websocket`, `github.com/vmihailenco/msgpack/v5`

---

## Rust

### Installation

```toml
# Cargo.toml
[dependencies]
xgen-sandbox = "0.1"
tokio = { version = "1", features = ["full"] }
```

### Quick Start

```rust
use xgen_sandbox::{XgenClient, CreateSandboxOptions};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = XgenClient::new("xgen_dev_key", "http://localhost:8080");

    let sandbox = client.create_sandbox(CreateSandboxOptions {
        template: Some("base".into()),
        ports: Some(vec![3000]),
        ..Default::default()
    }).await?;

    // Execute a command
    let result = sandbox.exec("echo", None).await?;
    println!("{}", result.stdout); // "hello\n"

    // File operations
    sandbox.write_file("hello.txt", b"Hello!").await?;
    let content = sandbox.read_text_file("hello.txt").await?;
    let files = sandbox.list_dir(".").await?;

    // Watch files
    let handle = sandbox.watch_files(".", |event| {
        println!("{}: {}", event.event_type, event.path);
    }).await?;
    // handle.cancel().await to stop

    // Port events
    let port_handle = sandbox.on_port_open(|port| {
        println!("Port {} opened", port);
    }).await?;

    // Cleanup
    sandbox.destroy().await?;
    Ok(())
}
```

### Client Methods

```rust
impl XgenClient {
    pub fn new(api_key: &str, agent_url: &str) -> Self
    pub async fn create_sandbox(&self, options: CreateSandboxOptions) -> Result<Sandbox, Error>
    pub async fn get_sandbox(&self, id: &str) -> Result<Sandbox, Error>
    pub async fn list_sandboxes(&self) -> Result<Vec<SandboxInfo>, Error>
}
```

### Sandbox Methods

```rust
impl Sandbox {
    pub async fn status(&self) -> SandboxStatus
    pub fn get_preview_url(&self, port: u16) -> Option<&String>
    pub async fn exec(&self, command: &str, options: Option<ExecOptions>) -> Result<ExecResult, Error>
    pub async fn read_file(&self, path: &str) -> Result<Vec<u8>, Error>
    pub async fn read_text_file(&self, path: &str) -> Result<String, Error>
    pub async fn write_file(&self, path: &str, content: &[u8]) -> Result<(), Error>
    pub async fn list_dir(&self, path: &str) -> Result<Vec<FileInfo>, Error>
    pub async fn remove_file(&self, path: &str, recursive: bool) -> Result<(), Error>
    pub async fn watch_files(&self, path: &str, callback: impl Fn(FileEvent) + Send + Sync + 'static) -> Result<WatchHandle, Error>
    pub async fn on_port_open(&self, callback: impl Fn(u16) + Send + Sync + 'static) -> Result<WatchHandle, Error>
    pub async fn keep_alive(&self) -> Result<(), Error>
    pub async fn destroy(&self) -> Result<(), Error>
}
```

**Dependencies:** `tokio`, `reqwest`, `tokio-tungstenite`, `rmp-serde`, `serde`

---

## Browser Components

### Installation

```bash
npm install @xgen-sandbox/browser
```

### SandboxTerminal

Interactive terminal using xterm.js with the binary WebSocket protocol.

```tsx
import { SandboxTerminal } from "@xgen-sandbox/browser";

<SandboxTerminal
  wsUrl="http://localhost:8080/api/v1/sandboxes/abc123/ws"
  token="jwt-token-here"
  cols={120}
  rows={40}
  fontSize={14}
  onConnect={() => console.log("connected")}
  onDisconnect={() => console.log("disconnected")}
/>
```

### SandboxPreview

Sandbox web service preview via iframe.

```tsx
import { SandboxPreview } from "@xgen-sandbox/browser";

<SandboxPreview
  url="https://sbx-abc123-3000.preview.example.com"
  title="My App"
  showUrlBar={true}
  onLoad={() => console.log("loaded")}
/>
```

### SandboxDesktop

VNC desktop viewer using noVNC.

```tsx
import { SandboxDesktop } from "@xgen-sandbox/browser";

<SandboxDesktop
  vncUrl="https://sbx-abc123-6080.preview.example.com"
  viewOnly={false}
  scaleViewport={true}
  onConnect={() => console.log("VNC connected")}
  onDisconnect={({ clean }) => console.log("VNC disconnected", clean)}
/>
```

The component automatically converts `https://` to `wss://` for the WebSocket connection.

### SandboxFiles

File browser with directory navigation, editing, and deletion.

```tsx
import { SandboxFiles } from "@xgen-sandbox/browser";

<SandboxFiles
  listDir={async (path) => sandbox.listDir(path)}
  readFile={async (path) => sandbox.readTextFile(path)}
  writeFile={async (path, content) => sandbox.writeFile(path, content)}
  deleteFile={async (path) => sandbox.removeFile(path)}
  initialPath="."
  onFileSelect={(path, content) => console.log("selected:", path)}
/>
```

---

## Common Patterns

### GUI Sandbox with VNC

```typescript
const sandbox = await client.createSandbox({
  template: "gui",
  gui: true,
});

// VNC URL is in the response
console.log(sandbox.info.vncUrl);
// → "https://sbx-abc123-6080.preview.example.com"
```

### Web Server with Preview

```typescript
const sandbox = await client.createSandbox({
  template: "nodejs",
  ports: [3000],
});

await sandbox.writeFile("server.js", `
  require('http').createServer((_, res) => {
    res.end('Hello!');
  }).listen(3000);
`);

sandbox.onPortOpen((port) => {
  console.log(`Preview: ${sandbox.getPreviewUrl(port)}`);
});

sandbox.exec("node server.js");
```

### Timeout Management

```typescript
// Create with 5-minute timeout
const sandbox = await client.createSandbox({ timeout_seconds: 300 });

// Extend by the default duration (1 hour) at any time
await sandbox.keepAlive();
```

---

## Command Execution

All SDKs wrap commands with `sh -c`, so shell features (pipes, redirects, environment variable expansion) work out of the box.

### exec vs execStream

| | `exec` | `execStream` |
|---|---|---|
| **Use case** | Short commands that complete quickly | Long-running processes, servers |
| **Returns** | `ExecResult` with stdout/stderr after completion | Async iterator of `ExecEvent` |
| **Blocking** | Yes — waits until the process exits | No — yields events as they arrive |
| **Background processes** | Use `nohup cmd &` pattern | `break` from iterator to detach |

```typescript
// exec: good for quick commands
const result = await sandbox.exec("ls -la");

// execStream: good for long-running processes
for await (const event of sandbox.execStream("npm start")) {
  if (event.type === "stdout" && event.data?.includes("ready")) {
    break; // Server is up, stop consuming events
  }
}
```

```python
# exec: good for quick commands
result = await sandbox.exec("ls -la")

# exec_stream: good for long-running processes
async for event in sandbox.exec_stream("npm start"):
    if event.type == "stdout" and "ready" in (event.data or ""):
        break
```

---

## Error Handling

### TypeScript

```typescript
try {
  const sandbox = await client.createSandbox({ template: "nodejs" });
  const result = await sandbox.exec("node -v");
} catch (err) {
  if (err.message.includes("Auth failed")) {
    // Invalid API key or agent unreachable
  } else if (err.message.includes("not found")) {
    // Sandbox does not exist
  } else if (err.message.includes("timeout")) {
    // Sandbox did not start in time, or command timed out
  } else if (err.message.includes("WebSocket not connected")) {
    // WebSocket disconnected — sandbox may have been destroyed
  }
}
```

**Common errors:**

| Error message | Cause | Solution |
|---|---|---|
| `Auth failed (POST .../auth/token): 401` | Invalid API key | Check `apiKey` value |
| `Create sandbox failed (500): ...` | Server-side error | Check agent logs |
| `Get sandbox failed: sandbox 'xxx' not found (404)` | Sandbox expired or destroyed | Create a new sandbox |
| `Exec timeout` | Command did not complete in time | Increase `timeout` option |
| `WebSocket request timeout after 30000ms` | Sidecar unresponsive | Sandbox may be overloaded or crashed |
| `WebSocket not connected` | Connection lost | Sandbox was destroyed or network issue |

### Python

```python
try:
    sandbox = await client.create_sandbox(template="python")
    result = await sandbox.exec("python3 -V")
except RuntimeError as e:
    # HTTP API errors (create, get, exec, delete)
    print(f"API error: {e}")
except TimeoutError as e:
    # Sandbox startup timeout or WebSocket request timeout
    print(f"Timeout: {e}")
except ConnectionError as e:
    # WebSocket disconnected
    print(f"Connection lost: {e}")
```

### Go

```go
sandbox, err := client.CreateSandbox(ctx, opts)
if err != nil {
    // Errors are wrapped with context, use errors.Is/errors.As or string matching
    log.Fatalf("create failed: %v", err)
}

result, err := sandbox.Exec(ctx, "go version")
if err != nil {
    // err contains sandbox ID and operation context
    log.Printf("exec failed: %v", err)
}
```

### Rust

```rust
match client.create_sandbox(opts).await {
    Ok(sandbox) => { /* use sandbox */ }
    Err(e) => eprintln!("Failed: {}", e),
}
```

---

## SDK Feature Matrix

| Feature | TypeScript | Python | Go | Rust |
|---|:---:|:---:|:---:|:---:|
| `exec` (sync) | REST | REST | WebSocket | REST |
| `execStream` | WebSocket | WebSocket | WebSocket | — |
| `openTerminal` | WebSocket | WebSocket | WebSocket | — |
| File operations | WebSocket | WebSocket | WebSocket | WebSocket |
| `watchFiles` | WebSocket | WebSocket | WebSocket | WebSocket |
| `onPortOpen` | WebSocket | WebSocket | WebSocket | WebSocket |
| `keepAlive` | REST | REST | REST | REST |
| Auto-reconnect | Yes | Yes | Yes | No |
| Browser support | Yes | — | — | — |
