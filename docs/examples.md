# Examples Guide

This guide explains how to run each example in the `examples/` directory. All examples require a running xgen-sandbox agent.

---

## Prerequisites

### 1. Start the Agent

```bash
# Build images and deploy to local Kind cluster (one-liner)
make build-images && make dev-cluster && make dev-deploy

# Verify
curl http://localhost:8080/healthz
# ok
```

See [local-testing.md](local-testing.md) for detailed setup instructions.

### 2. Build Runtime Images

Different examples require different runtime images. Build what you need:

```bash
# Base runtime (required by all basic-exec examples)
# Already built by `make build-images`

# Node.js runtime (required by web-preview examples)
docker build --no-cache -t ghcr.io/xgen-sandbox/runtime-nodejs:latest ./runtime/nodejs
kind load docker-image ghcr.io/xgen-sandbox/runtime-nodejs:latest --name xgen-sandbox

# GUI runtime (required by gui-desktop and browser-components)
docker build --no-cache -t ghcr.io/xgen-sandbox/runtime-gui:latest ./runtime/gui
kind load docker-image ghcr.io/xgen-sandbox/runtime-gui:latest --name xgen-sandbox
```

### 3. Environment Variables

All examples read these environment variables with sensible defaults:

| Variable | Default | Description |
|----------|---------|-------------|
| `API_KEY` | `xgen_dev_key` | API key for authentication |
| `AGENT_URL` | `http://localhost:8080` | Agent server URL |

Override if your setup differs:

```bash
export API_KEY="your_key"
export AGENT_URL="http://your-host:8080"
```

---

## Examples Overview

| Example | Language/Framework | Features Demonstrated |
|---------|-------------------|----------------------|
| [basic-exec](#basic-exec-typescript) | TypeScript | Command execution, file ops, streaming |
| [basic-exec-python](#basic-exec-python) | Python | Command execution, file ops |
| [basic-exec-go](#basic-exec-go) | Go | Command execution, file ops |
| [basic-exec-rust](#basic-exec-rust) | Rust | Command execution, file ops |
| [web-preview](#web-preview-typescript) | TypeScript | Web server, port detection, preview URL |
| [web-preview-python](#web-preview-python) | Python | Web server, port detection, preview URL |
| [gui-desktop](#gui-desktop) | TypeScript | GUI sandbox, VNC access |
| [browser-components](#browser-components) | React (Vite) | Terminal, Preview, Desktop, Files components |

---

## basic-exec (TypeScript)

Creates a sandbox, runs commands, performs file operations, and demonstrates streaming output.

```bash
cd examples/basic-exec
npm install
npx tsx main.ts
```

**Expected output:**

```
Creating sandbox...
Sandbox created: a1b2c3d4 (status: running)

Running: echo 'Hello from xgen-sandbox!'
Exit code: 0
Stdout: Hello from xgen-sandbox!

Running: uname -a
System: Linux sbx-a1b2c3d4 5.15.0 ...

Writing file...
File content: Hello, World!

Listing workspace:
  - hello.txt (14 bytes)

Streaming: for i in 1 2 3; do echo $i; sleep 0.5; done
  [stdout] 1
  [stdout] 2
  [stdout] 3
  [exit] code=0

Destroying sandbox...
Done.
```

---

## basic-exec-python

Same functionality as `basic-exec`, using the Python SDK.

**Requirements:** Python 3.10+

```bash
cd examples/basic-exec-python

# Create a virtual environment (recommended)
python3 -m venv .venv
source .venv/bin/activate

# Install dependencies (editable link to local SDK)
pip install -r requirements.txt

# Run
python main.py
```

**Expected output:**

```
Creating sandbox...
Sandbox created: a1b2c3d4 (status: running)

Running: echo 'Hello from xgen-sandbox!'
Exit code: 0
Stdout: Hello from xgen-sandbox!

Running: uname -a
System: Linux sbx-a1b2c3d4 5.15.0 ...

Writing file...
File content: Hello, World!

Listing workspace:
  - hello.txt (14 bytes)

Destroying sandbox...
Done.
```

---

## basic-exec-go

Same functionality as `basic-exec`, using the Go SDK.

**Requirements:** Go 1.22+

```bash
cd examples/basic-exec-go

# Download dependencies (uses replace directive for local SDK)
go mod tidy

# Run
go run main.go
```

> **Note:** The `go.mod` file contains a `replace` directive pointing to `../../sdks/go`. This allows using the local SDK without publishing it. Running `go mod tidy` will resolve transitive dependencies and generate a `go.sum` file.

**Expected output:**

```
Creating sandbox...
Sandbox created: a1b2c3d4 (status: running)

Running: echo 'Hello from xgen-sandbox!'
Exit code: 0
Stdout: Hello from xgen-sandbox!

Running: uname -a
System: Linux sbx-a1b2c3d4 5.15.0 ...

Writing file...
File content: Hello, World!

Listing workspace:
  - hello.txt (14 bytes)

Destroying sandbox...
Done.
```

---

## basic-exec-rust

Same functionality as `basic-exec`, using the Rust SDK.

**Requirements:** Rust (latest stable), with `cargo`

```bash
cd examples/basic-exec-rust

# Build and run (first build will take longer due to dependency compilation)
cargo run
```

> **Note:** The `Cargo.toml` uses a `path` dependency (`../../sdks/rust`) to reference the local SDK. The first build compiles all SDK dependencies (tokio, reqwest, etc.) which may take a few minutes.

**Expected output:**

```
Creating sandbox...
Sandbox created: a1b2c3d4 (status: Running)

Running: echo 'Hello from xgen-sandbox!'
Exit code: 0
Stdout: Hello from xgen-sandbox!

Running: uname -a
System: Linux sbx-a1b2c3d4 5.15.0 ...

Writing file...
File content: Hello, World!

Listing workspace:
  - hello.txt (14 bytes)

Destroying sandbox...
Done.
```

---

## web-preview (TypeScript)

Deploys a Node.js HTTP server inside a sandbox and exposes it via a preview URL.

**Requirements:** Node.js 18+, `runtime-nodejs` image loaded into Kind

```bash
cd examples/web-preview
npm install
npx tsx main.ts
```

Once the server is running, open the printed preview URL in your browser. Press `Ctrl+C` to stop.

> **DNS required:** Preview URLs use wildcard subdomains (e.g. `sbx-<id>-3000.preview.localhost`). See [DNS Setup](local-testing.md#dns-setup-for-preview-urls) for configuration.

---

## web-preview-python

Same functionality as `web-preview`, using the Python SDK.

**Requirements:** Python 3.10+, `runtime-nodejs` image loaded into Kind

```bash
cd examples/web-preview-python

python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

python main.py
```

The script starts a Node.js HTTP server in the sandbox, prints the preview URL, and waits for `Ctrl+C`.

---

## gui-desktop

Creates a sandbox with a graphical desktop environment accessible via VNC in your browser.

**Requirements:** Node.js 18+, `runtime-gui` image loaded into Kind

```bash
cd examples/gui-desktop
npm install
npx tsx main.ts
```

**Expected output:**

```
Creating GUI sandbox with VNC...
Sandbox: a1b2c3d4
VNC URL: http://sbx-a1b2c3d4-6080.preview.localhost:8080
WS URL: http://localhost:8080/api/v1/sandboxes/a1b2c3d4/ws

Waiting for desktop to start...
Launching xterm...
xterm launched (exit code: 0)
Display info:
  name of display:    :0
  ...

Desktop is ready!
Open VNC in browser: http://sbx-a1b2c3d4-6080.preview.localhost:8080
Press Ctrl+C to stop.
```

Open the VNC URL in your browser to see the graphical desktop with xterm running. The sandbox stays alive with periodic keep-alive signals until you press `Ctrl+C`.

---

## browser-components

A Vite React application that demonstrates all four `@xgen-sandbox/browser` components wired to a live sandbox.

**Requirements:** Node.js 18+, `runtime-gui` image loaded into Kind

### Setup

```bash
# 1. Build the TypeScript SDK (if not already built)
cd sdks/typescript
npm install
npm run build

# 2. Build the browser component library
cd ../../browser
npm install
npm run build

# 3. Install and run the example
cd ../examples/browser-components
npm install
npm run dev
```

### Usage

Open `http://localhost:5173` in your browser. The app will:

1. Authenticate with the agent and obtain a JWT token
2. Create a GUI sandbox with port 3000 exposed
3. Start a simple HTTP server inside the sandbox

Switch between tabs to interact with each component:

| Tab | Component | Description |
|-----|-----------|-------------|
| **Terminal** | `SandboxTerminal` | Interactive shell (xterm.js over WebSocket) |
| **Preview** | `SandboxPreview` | Web preview iframe with URL bar |
| **Desktop** | `SandboxDesktop` | VNC graphical desktop (noVNC) |
| **Files** | `SandboxFiles` | File browser with read/write/delete |

> **Note:** The Desktop tab requires DNS setup for VNC URLs. The Preview tab requires DNS setup for preview URLs. See [DNS Setup](local-testing.md#dns-setup-for-preview-urls).

---

## Troubleshooting

### Common Issues

| Problem | Solution |
|---------|----------|
| `connection refused` on localhost:8080 | Ensure the agent is running: `kubectl get pods -n xgen-system` |
| Sandbox stays in `starting` state | Check sidecar logs: `kubectl logs sbx-<id> -n xgen-sandboxes -c sidecar` |
| `ImagePullBackOff` on sandbox pod | Load the required runtime image into Kind (see [Build Runtime Images](#2-build-runtime-images)) |
| Preview URL returns `502` | Server in sandbox must bind to `0.0.0.0`, not `127.0.0.1` |
| Preview URL returns `Could not resolve host` | Set up wildcard DNS for `*.preview.localhost` (see [DNS Setup](local-testing.md#dns-setup-for-preview-urls)) |
| Python: `ModuleNotFoundError: xgen_sandbox` | Run `pip install -r requirements.txt` in the example directory |
| Go: `cannot find module` | Run `go mod tidy` in the example directory |
| Rust: long first build time | Normal - tokio and reqwest compile from source on first build |
| browser-components: module not found | Build SDKs first (`npm run build` in `sdks/typescript/` and `browser/`) |

### Cleanup

```bash
# Delete all sandbox pods
kubectl delete pods --all -n xgen-sandboxes

# Teardown entire local cluster
make dev-teardown
```

For more debugging tips, see [local-testing.md](local-testing.md#10-troubleshooting).
