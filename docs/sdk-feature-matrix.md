# SDK Feature Matrix

| Feature | TypeScript | Python | Go | Rust |
|---------|-----------|--------|-----|------|
| **Execution** | | | | |
| exec() (REST) | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| execStream() (WebSocket) | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: |
| openTerminal() (WebSocket) | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: |
| **File Operations** | | | | |
| readFile() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| writeFile() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| listDir() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| removeFile() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| **Events** | | | | |
| watchFiles() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| onPortOpen() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| **Capabilities** | | | | |
| capabilities (sudo, git-ssh, browser) | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| **Lifecycle** | | | | |
| keepAlive() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| destroy() | :white_check_mark: | :white_check_mark: | :white_check_mark: | :white_check_mark: |
| **Resilience** | | | | |
| Auto-reconnect | :white_check_mark: | :white_check_mark: | :white_check_mark: | :x: |

## Installation

**TypeScript:**
```bash
npm install @xgen-sandbox/sdk
```

**Python:**
```bash
pip install xgen-sandbox
```

**Go:**
```bash
go get github.com/xgen-sandbox/sdk-go
```

**Rust:**
```toml
[dependencies]
xgen-sandbox = "0.1"
```
