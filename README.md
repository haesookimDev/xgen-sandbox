# xgen-sandbox

Kubernetes-based code execution sandbox platform. Run code, preview web services, and interact with GUI applications in isolated containers.

## Features

- **Isolated Execution** — Each sandbox runs in a dedicated K8s pod with security contexts, network policies, and resource limits
- **Multi-runtime** — Base (Ubuntu), Node.js, Python, Go, GUI (Xvfb + VNC)
- **Web Preview** — Expose sandbox ports via dynamic subdomain routing (`sbx-{id}-{port}.preview.example.com`)
- **Interactive Terminal** — Full PTY support over WebSocket with xterm.js
- **GUI Desktop** — VNC access to graphical applications via noVNC in the browser
- **File Operations** — Read, write, list, delete, and watch files over WebSocket
- **Multi-language SDKs** — TypeScript, Python, Go, Rust
- **React Components** — `<SandboxPreview>`, `<SandboxTerminal>`, `<SandboxDesktop>`, `<SandboxFiles>`
- **Auth & RBAC** — JWT tokens with admin/user/viewer roles
- **Warm Pool** — Pre-created pods for sub-second sandbox startup
- **Observability** — Prometheus metrics, structured logging (slog), audit logs
- **Production Ready** — Helm chart with Ingress, HPA, PDB, NetworkPolicy, ResourceQuota

## Architecture

```
SDK / Browser ──▶ Agent (REST + WS) ──▶ K8s API
                      │                       │
                      ▼                       ▼
               Preview Router           Sandbox Pod
               (dynamic proxy)     ┌──────────────────┐
                                   │ Sidecar (WS 9000)│
                                   │ Runtime (user code)│
                                   │ VNC (optional 6080)│
                                   └──────────────────┘
```

See [docs/architecture.md](docs/architecture.md) for details.

## Quick Start

### Prerequisites

- Go 1.22+
- Docker
- [Kind](https://kind.sigs.k8s.io/)
- [Helm](https://helm.sh/)
- Node.js 18+ (for SDKs and browser components)

### 1. Build

```bash
# Build agent and sidecar binaries
make build

# Build Docker images
make build-images
```

### 2. Create Local Cluster

```bash
# Create Kind cluster and load images
make dev-cluster

# Deploy with Helm
make dev-deploy
```

### 3. Use the SDK

```typescript
import { XgenClient } from "@xgen-sandbox/sdk";

const client = new XgenClient({
  apiKey: "xgen_dev_key",
  agentUrl: "http://localhost:8080",
});

const sandbox = await client.createSandbox({ template: "nodejs" });

const result = await sandbox.exec("node", {
  args: ["-e", "console.log('Hello!')"],
});
console.log(result.stdout); // "Hello!\n"

await sandbox.destroy();
```

See [docs/sdk-guide.md](docs/sdk-guide.md) for all SDK languages.

## Project Structure

```
xgen-sandbox/
├── agent/              # Go — Control plane (REST API, K8s pod management, WS proxy)
├── sidecar/            # Go — In-pod helper (exec, filesystem, port detection)
├── runtime/            # Dockerfiles — base, nodejs, python, go, gui
├── sdks/
│   ├── typescript/     # @xgen-sandbox/sdk
│   ├── python/         # xgen-sandbox (PyPI)
│   ├── go/             # github.com/xgen-sandbox/sdk-go
│   └── rust/           # xgen-sandbox (crates.io)
├── browser/            # @xgen-sandbox/browser — React components
├── deploy/
│   ├── helm/           # Helm chart
│   └── dev/            # Kind cluster config
├── examples/           # SDK usage examples
└── docs/               # Documentation
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System design, pod structure, protocol |
| [API Reference](docs/api-reference.md) | REST API endpoints and WebSocket protocol |
| [SDK Guide](docs/sdk-guide.md) | TypeScript, Python, Go, Rust SDK usage |
| [Deployment](docs/deployment.md) | Local dev, Helm chart, production config |
| [Security](docs/security.md) | Auth, RBAC, network policies, pod security |

## Development

```bash
make build          # Build binaries
make build-images   # Build Docker images
make test           # Run Go tests
make lint           # Run go vet
make tidy           # Run go mod tidy
make dev-cluster    # Create Kind cluster
make dev-deploy     # Deploy to Kind
make dev-reload     # Rebuild and restart
make dev-teardown   # Delete Kind cluster
```

## License

MIT
