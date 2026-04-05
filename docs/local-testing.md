# Local Testing Guide

This guide covers setting up xgen-sandbox locally from scratch, verifying the server, and testing each SDK.

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| **Go** | 1.22+ | Build agent and sidecar |
| **Docker** | Latest | Build container images |
| **[Kind](https://kind.sigs.k8s.io/)** | Latest | Local Kubernetes cluster |
| **[Helm](https://helm.sh/)** | 3.x | Deploy to Kubernetes |
| **kubectl** | Latest | Cluster management |
| **Node.js** | 18+ | TypeScript SDK and examples |
| **Python** | 3.10+ | Python SDK (optional) |
| **Rust** | Latest stable | Rust SDK (optional) |

Install Kind and Helm if needed:

```bash
# Kind
go install sigs.k8s.io/kind@latest

# Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
```

---

## 1. Build & Deploy

### One-liner (from project root)

```bash
make build-images && make dev-cluster && make dev-deploy
```

### Step by step

```bash
# 1. Build Docker images (agent, sidecar, runtime-base)
make build-images

# 2. Create Kind cluster + load images into it
make dev-cluster

# 3. Deploy with Helm
make dev-deploy
```

What this creates:

| Resource | Namespace | Description |
|----------|-----------|-------------|
| Deployment `xgen-agent` | `xgen-system` | Agent server (1 replica) |
| Service `xgen-agent` | `xgen-system` | ClusterIP on port 8080 |
| ServiceAccount + RBAC | `xgen-system` | K8s API permissions |
| NetworkPolicy | `xgen-sandboxes` | Sandbox isolation rules |
| ResourceQuota | `xgen-sandboxes` | Pod/CPU/memory limits |

### Verify

```bash
# Check agent is running
kubectl get pods -n xgen-system
# NAME                          READY   STATUS    AGE
# xgen-agent-xxxxxxxxx-xxxxx   1/1     Running   30s

# Check logs
kubectl logs -n xgen-system deployment/xgen-agent
```

### Access the Agent

The Kind cluster maps NodePort 30080 to host port 8080 (see `deploy/dev/kind-config.yaml`).
The agent is exposed as a NodePort service, so it's available at `localhost:8080` without port-forwarding.

```bash
# Quick health check
curl http://localhost:8080/healthz
# ok
```

> **Fallback**: If NodePort isn't working, use port-forward instead:
> ```bash
> kubectl port-forward -n xgen-system svc/xgen-agent 8080:8080
> ```

### DNS Setup for Preview URLs

Preview URLs use wildcard subdomains like `sbx-<id>-3000.preview.localhost:8080`.
These must resolve to `127.0.0.1` so the browser can reach the agent, which reverse-proxies to the sandbox pod.

#### macOS

On modern macOS, `*.localhost` already resolves to `127.0.0.1` by default (RFC 6761). Verify:

```bash
dscacheutil -q host -a name sbx-test-3000.preview.localhost
# Should show: ip_address: 127.0.0.1
```

If it doesn't resolve, add a local DNS resolver:

```bash
# Create resolver directory if it doesn't exist
sudo mkdir -p /etc/resolver

# Add wildcard rule for preview.localhost
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/localhost
```

Or add entries to `/etc/hosts` (limited — no wildcard support, must add per-sandbox):

```bash
# Not recommended: you'd need to add a line for every sandbox
echo "127.0.0.1 sbx-abc123-3000.preview.localhost" | sudo tee -a /etc/hosts
```

**Recommended approach for macOS**: Use [dnsmasq](https://formulae.brew.sh/formula/dnsmasq) for true wildcard DNS:

```bash
# Install
brew install dnsmasq

# Route all *.localhost to 127.0.0.1
echo "address=/localhost/127.0.0.1" >> $(brew --prefix)/etc/dnsmasq.conf

# Start as a service
sudo brew services start dnsmasq

# Point macOS at dnsmasq for .localhost domains
sudo mkdir -p /etc/resolver
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/localhost

# Verify
dig sbx-test-3000.preview.localhost @127.0.0.1
# Should return 127.0.0.1
```

#### Linux

Add a wildcard DNS entry using systemd-resolved or dnsmasq:

**Option A: systemd-resolved (Ubuntu 18.04+)**

```bash
# Most Linux systems resolve *.localhost to 127.0.0.1 by default. Verify:
getent hosts sbx-test-3000.preview.localhost

# If not working, add to /etc/hosts (no wildcard support):
echo "127.0.0.1 sbx-test-3000.preview.localhost" | sudo tee -a /etc/hosts
```

**Option B: dnsmasq**

```bash
sudo apt install dnsmasq

# Route all *.localhost to 127.0.0.1
echo "address=/localhost/127.0.0.1" | sudo tee /etc/dnsmasq.d/localhost.conf
sudo systemctl restart dnsmasq

# Point /etc/resolv.conf to dnsmasq
echo "nameserver 127.0.0.1" | sudo tee /etc/resolv.conf
```

#### Verify DNS

```bash
# Should all resolve to 127.0.0.1:
curl -s -o /dev/null -w "%{http_code}" http://sbx-test-3000.preview.localhost:8080
# 404 is expected (no sandbox with that ID), but it means DNS + routing works

# If you get "Could not resolve host", DNS is not set up correctly
```

#### How Preview Routing Works

```
Browser → sbx-<id>-3000.preview.localhost:8080
    ↓ DNS resolves to 127.0.0.1
localhost:8080 → Kind NodePort 30080 → Agent pod
    ↓ Agent checks Host header
    ↓ Matches *.preview.localhost → Preview Router
    ↓ Parses subdomain: sandbox ID + port
    ↓ Reverse proxy to pod_ip:3000
    → Sandbox runtime container
```

---

## 2. Test with curl

Before using SDKs, verify the API works directly.

### Create a sandbox

```bash
curl -s -X POST http://localhost:8080/api/v1/sandboxes \
  -H "Authorization: ApiKey xgen_dev_key" \
  -H "Content-Type: application/json" \
  -d '{"template": "base", "timeout_seconds": 300}' | jq .
```

Response:

```json
{
  "id": "a1b2c3d4",
  "status": "starting",
  "template": "base",
  "ws_url": "http://localhost:8080/api/v1/sandboxes/a1b2c3d4/ws",
  "created_at": "...",
  "expires_at": "..."
}
```

Save the sandbox ID:

```bash
SANDBOX_ID="a1b2c3d4"  # replace with actual ID
```

### Watch sandbox pod start

```bash
kubectl get pods -n xgen-sandboxes -w
# sbx-a1b2c3d4   0/2   ContainerCreating   ...
# sbx-a1b2c3d4   2/2   Running             ...
```

### Check sandbox status

Wait until the pod is Running, then:

```bash
curl -s http://localhost:8080/api/v1/sandboxes/$SANDBOX_ID \
  -H "Authorization: ApiKey xgen_dev_key" | jq .status
# "running"
```

### Execute a command

```bash
curl -s -X POST http://localhost:8080/api/v1/sandboxes/$SANDBOX_ID/exec \
  -H "Authorization: ApiKey xgen_dev_key" \
  -H "Content-Type: application/json" \
  -d '{"command": "echo", "args": ["Hello from xgen-sandbox!"]}' | jq .
```

Response:

```json
{
  "exit_code": 0,
  "stdout": "Hello from xgen-sandbox!\n",
  "stderr": ""
}
```

### Delete the sandbox

```bash
curl -s -X DELETE http://localhost:8080/api/v1/sandboxes/$SANDBOX_ID \
  -H "Authorization: ApiKey xgen_dev_key" -w "%{http_code}\n"
# 204
```

### Check Prometheus metrics

```bash
curl -s http://localhost:8080/metrics | grep xgen_
# xgen_http_requests_total{method="POST",path="/api/v1/sandboxes",status="201"} 1
# xgen_sandboxes_active 0
# xgen_sandbox_create_total 1
# xgen_sandbox_delete_total 1
```

---

## 3. TypeScript SDK

### Build the SDK

```bash
cd sdks/typescript
npm install
npm run build
```

### Link for local testing

```bash
# Register the SDK as a global link
cd sdks/typescript
npm link

# Link it into the example
cd ../../examples/basic-exec
npm link @xgen-sandbox/sdk
```

### Run the example

```bash
# Make sure port-forward is running in another terminal
cd examples/basic-exec
npx tsx main.ts
```

Expected output:

```
Creating sandbox...
Sandbox created: a1b2c3d4 (status: starting)

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

### Run the web preview example

```bash
cd examples/web-preview
npm link @xgen-sandbox/sdk
npx tsx main.ts
```

### Custom test script

```typescript
// test.ts
import { XgenClient } from "@xgen-sandbox/sdk";

const client = new XgenClient({
  apiKey: "xgen_dev_key",
  agentUrl: "http://localhost:8080",
});

const sandbox = await client.createSandbox({ template: "base" });
console.log("Created:", sandbox.id);

const result = await sandbox.exec("cat /etc/os-release");
console.log(result.stdout);

await sandbox.destroy();
console.log("Destroyed.");
```

```bash
npx tsx test.ts
```

---

## 4. Python SDK

### Install

```bash
cd sdks/python
pip install -e .
```

This installs the SDK in editable mode with dependencies: `httpx`, `websockets`, `msgpack`.

### Test script

```python
# test_sdk.py
import asyncio
from xgen_sandbox import XgenClient

async def main():
    async with XgenClient("xgen_dev_key", "http://localhost:8080") as client:
        # Create sandbox
        sandbox = await client.create_sandbox(template="base")
        print(f"Created: {sandbox.id}")

        # Execute command
        result = await sandbox.exec("echo hello from python")
        print(f"stdout: {result.stdout}")
        print(f"exit_code: {result.exit_code}")

        # File operations
        await sandbox.write_file("test.txt", "Hello from Python!\n")
        content = await sandbox.read_text_file("test.txt")
        print(f"file content: {content}")

        # List directory
        files = await sandbox.list_dir(".")
        for f in files:
            print(f"  {'d' if f.is_dir else '-'} {f.name} ({f.size} bytes)")

        # Cleanup
        await sandbox.destroy()
        print("Destroyed.")

asyncio.run(main())
```

```bash
python3 test_sdk.py
```

---

## 5. Go SDK

### Test program

Create a temporary test file:

```bash
mkdir -p /tmp/xgen-go-test && cd /tmp/xgen-go-test

cat > main.go << 'EOF'
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
		Template: "base",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Created: %s\n", sandbox.ID)

	result, err := sandbox.Exec(ctx, "echo hello from go")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("stdout: %s", result.Stdout)

	files, err := sandbox.ListDir(ctx, ".")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		fmt.Printf("  %s (%d bytes)\n", f.Name, f.Size)
	}

	if err := sandbox.Destroy(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Destroyed.")
}
EOF

go mod init xgen-go-test
```

Point to the local SDK using `replace`:

```bash
# Replace the module path with the local SDK directory
go mod edit -replace github.com/xgen-sandbox/sdk-go=$HOME/Desktop/opensource/xgen-sandbox/sdks/go
go mod tidy
go run main.go
```

---

## 6. Rust SDK

### Test program

```bash
mkdir -p /tmp/xgen-rust-test && cd /tmp/xgen-rust-test

cargo init --name xgen-rust-test
```

Edit `Cargo.toml`:

```toml
[dependencies]
xgen-sandbox = { path = "~/Desktop/opensource/xgen-sandbox/sdks/rust" }
tokio = { version = "1", features = ["full"] }
```

Edit `src/main.rs`:

```rust
use xgen_sandbox::{XgenClient, CreateSandboxOptions};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = XgenClient::new("xgen_dev_key", "http://localhost:8080");

    let sandbox = client.create_sandbox(CreateSandboxOptions {
        template: Some("base".into()),
        ..Default::default()
    }).await?;
    println!("Created: {}", sandbox.id);

    let result = sandbox.exec("echo hello from rust", None).await?;
    println!("stdout: {}", result.stdout);

    sandbox.destroy().await?;
    println!("Destroyed.");
    Ok(())
}
```

```bash
cargo run
```

---

## 7. Browser Components

### Build

```bash
cd browser
npm install
npm run build
```

### Link for local testing

```bash
cd browser
npm link

# In your React app:
npm link @xgen-sandbox/browser
```

### Test in a React app

```tsx
import { SandboxTerminal, SandboxPreview, SandboxDesktop } from "@xgen-sandbox/browser";

function App() {
  const sandboxId = "your-sandbox-id";
  const token = "your-jwt-token";

  return (
    <div style={{ display: "flex", height: "100vh" }}>
      {/* Interactive terminal */}
      <SandboxTerminal
        wsUrl={`http://localhost:8080/api/v1/sandboxes/${sandboxId}/ws`}
        token={token}
        style={{ flex: 1 }}
      />

      {/* Web preview (requires sandbox with ports exposed) */}
      <SandboxPreview
        url={`http://sbx-${sandboxId}-3000.preview.localhost`}
        showUrlBar
        style={{ flex: 1 }}
      />

      {/* VNC desktop (requires sandbox with gui: true) */}
      <SandboxDesktop
        vncUrl={`http://sbx-${sandboxId}-6080.preview.localhost`}
        style={{ flex: 1 }}
      />
    </div>
  );
}
```

To get a JWT token for the terminal component:

```bash
curl -s -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"api_key": "xgen_dev_key"}' | jq -r .token
```

---

## 8. Development Workflow

### After code changes

```bash
# Rebuild images, reload into Kind, restart agent
make dev-reload
```

This runs:
1. `make build-images` — Rebuild Docker images
2. `kind load docker-image ...` — Load new images into Kind
3. `kubectl rollout restart deployment/xgen-agent` — Restart the agent

### Rebuild only the SDK

```bash
# TypeScript
cd sdks/typescript && npm run build

# Python (editable mode, changes apply immediately)
# No rebuild needed

# Go (no build step, `go run` compiles on the fly)
# No rebuild needed

# Rust
cd sdks/rust && cargo build
```

### Monitor sandbox pods

```bash
# Watch pods in real-time
kubectl get pods -n xgen-sandboxes -w

# View sidecar logs for a specific sandbox
kubectl logs sbx-<sandbox-id> -n xgen-sandboxes -c sidecar

# View runtime logs
kubectl logs sbx-<sandbox-id> -n xgen-sandboxes -c runtime
```

---

## 9. Additional Runtime Images

By default only `runtime-base` is built. For Node.js, Python, Go, or GUI sandboxes:

```bash
# Build additional runtime images
docker build -t ghcr.io/xgen-sandbox/runtime-nodejs:latest ./runtime/nodejs
docker build -t ghcr.io/xgen-sandbox/runtime-python:latest ./runtime/python
docker build -t ghcr.io/xgen-sandbox/runtime-go:latest ./runtime/go
docker build -t ghcr.io/xgen-sandbox/runtime-gui:latest ./runtime/gui

# Load into Kind
kind load docker-image ghcr.io/xgen-sandbox/runtime-nodejs:latest --name xgen-sandbox
kind load docker-image ghcr.io/xgen-sandbox/runtime-python:latest --name xgen-sandbox
kind load docker-image ghcr.io/xgen-sandbox/runtime-go:latest --name xgen-sandbox
kind load docker-image ghcr.io/xgen-sandbox/runtime-gui:latest --name xgen-sandbox
```

Then you can create sandboxes with other templates:

```bash
curl -s -X POST http://localhost:8080/api/v1/sandboxes \
  -H "Authorization: ApiKey xgen_dev_key" \
  -H "Content-Type: application/json" \
  -d '{"template": "nodejs", "ports": [3000]}'
```

---

## 10. Troubleshooting

### Debug Script

Use the built-in debug script for quick diagnosis:

```bash
# Overview: agent status, sandbox pods, recent logs
./scripts/debug-sandbox.sh

# Debug a specific sandbox: pod status, capabilities, exec test, port scan
./scripts/debug-sandbox.sh <sandbox-id>

# Test exec via REST API
./scripts/debug-sandbox.sh exec <sandbox-id> echo hello
./scripts/debug-sandbox.sh exec <sandbox-id> node --version
```

### Common Issues

| Symptom | Cause | Solution |
|---------|-------|----------|
| `connection refused` on :8080 | Agent not accessible | Check Kind is running: `kind get clusters`. Check NodePort: `kubectl get svc -n xgen-system` |
| `sandbox service unavailable` (502) | No process listening on the requested port | Verify exec works first: `./scripts/debug-sandbox.sh exec <id> echo hello`. Check server binds to `0.0.0.0` not `127.0.0.1` |
| `Could not resolve host` on preview URL | DNS not set up for `*.preview.localhost` | See [DNS Setup](#dns-setup-for-preview-urls) above |
| `nsenter: Operation not permitted` | Sidecar missing capabilities | Sidecar must run as root with `CAP_SYS_ADMIN`. Check: `./scripts/debug-sandbox.sh <id>` |
| Pod stuck in `ImagePullBackOff` | Image not loaded into Kind | Run `make dev-cluster` or manually `kind load docker-image` |
| Pod stuck in `Pending` | ResourceQuota exceeded | Delete old sandboxes: `kubectl delete pods --all -n xgen-sandboxes` |
| Sandbox stays in `starting` | Sidecar readiness probe failing | Check sidecar logs: `kubectl logs sbx-<id> -n xgen-sandboxes -c sidecar` |
| `401 Unauthorized` | Wrong API key | Use `xgen_dev_key` (default dev key) |
| `429 Too Many Requests` | Rate limit hit | Wait 1 minute (120 req/min limit) |
| `exec` returns empty stdout | SDK not showing errors | Test with curl: `./scripts/debug-sandbox.sh exec <id> echo hello` |
| Kind cluster not found | Cluster deleted or not created | Run `make dev-cluster` |
| Helm install fails | Namespace conflict | Run `make dev-teardown` then `make dev-cluster` |

### Useful Debug Commands

```bash
# Agent logs (follow mode)
kubectl logs -n xgen-system deployment/xgen-agent -f

# Sidecar logs (shows WS connections, message flow, exec errors)
kubectl logs sbx-<id> -n xgen-sandboxes -c sidecar -f

# Runtime container logs
kubectl logs sbx-<id> -n xgen-sandboxes -c runtime

# Check sidecar capabilities (verify SYS_ADMIN is present)
kubectl exec -n xgen-sandboxes sbx-<id> -c sidecar -- cat /proc/1/status | grep Cap

# Describe a stuck pod
kubectl describe pod sbx-<id> -n xgen-sandboxes

# Check all resources in sandbox namespace
kubectl get all -n xgen-sandboxes

# Check resource quota usage
kubectl describe resourcequota -n xgen-sandboxes

# List Kind clusters
kind get clusters

# Check images loaded in Kind
docker exec xgen-sandbox-control-plane crictl images | grep xgen

# Restart agent without rebuilding
kubectl rollout restart deployment/xgen-agent -n xgen-system
```

### Clean Up Everything

```bash
# Delete all sandboxes
kubectl delete pods --all -n xgen-sandboxes

# Teardown the entire cluster
make dev-teardown
```

---

## Quick Reference

```bash
# === Full setup from scratch ===
make build-images && make dev-cluster && make dev-deploy

# === Access agent locally ===
kubectl port-forward -n xgen-system svc/xgen-agent 8080:8080

# === Run TypeScript example ===
cd sdks/typescript && npm install && npm run build && npm link
cd ../../examples/basic-exec && npm link @xgen-sandbox/sdk && npx tsx main.ts

# === Rebuild after changes ===
make dev-reload

# === Clean up ===
make dev-teardown
```
