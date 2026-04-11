# xgen-sandbox

Kubernetes-based code execution sandbox platform. Run code, preview web services, and interact with GUI applications in isolated containers.

## Features

- **Isolated Execution** — Each sandbox runs in a dedicated K8s pod with security contexts, network policies, and resource limits
- **Multi-runtime** — Base (Ubuntu), Node.js, Python, Go, GUI (Xvfb + VNC)
- **Custom Resources** — Per-sandbox CPU/memory limits via the API (`resources` field)
- **Web Preview** — Expose sandbox ports via dynamic subdomain routing (`sbx-{id}-{port}.preview.example.com`)
- **Interactive Terminal** — Full PTY support over WebSocket with xterm.js
- **GUI Desktop** — VNC access to graphical applications via noVNC in the browser
- **File Operations** — Read, write, list, delete, and watch files over WebSocket
- **Multi-language SDKs** — TypeScript, Python, Go, Rust with streaming exec and interactive terminal support
- **React Components** — `<SandboxPreview>`, `<SandboxTerminal>`, `<SandboxDesktop>`, `<SandboxFiles>`
- **Dashboard** — Next.js 15 admin UI with sandbox management, metrics, audit logs, and embedded terminal
- **Auth & RBAC** — JWT tokens with admin/user/viewer roles
- **Warm Pool** — Pre-created pods for sub-second startup, configurable per template
- **Auto-Reconnect** — WebSocket reconnection with exponential backoff across all SDKs
- **Observability** — Prometheus metrics, structured logging (slog), audit logs, Grafana dashboards
- **Production Ready** — Helm chart with Ingress, HPA, PDB, NetworkPolicy, ResourceQuota, Pod anti-affinity
- **CI/CD** — GitHub Actions with lint, test, security scanning (Trivy), E2E tests, automated releases

## Architecture

```
SDK / Browser ──> Agent (REST + WS) ──> K8s API
                      |                       |
                      v                       v
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

- Go 1.24+
- Docker
- [Kind](https://kind.sigs.k8s.io/) (for Kubernetes-based setup)
- [Helm](https://helm.sh/)
- Node.js 20+ (for dashboard and SDKs)

### Option A: Docker Compose

```bash
# Copy and configure environment
cp .env.example .env
# Edit .env — set API_KEY and JWT_SECRET

# Start agent + dashboard
docker compose up -d

# Dashboard: http://localhost:3000
# Agent API: http://localhost:8080
```

> Requires a running Kubernetes cluster (Kind, minikube, etc.) with kubeconfig at `~/.kube/config`.

### Option B: Kind Cluster

```bash
# Build Docker images
make build-images

# Create Kind cluster and load images
make dev-cluster

# Deploy with Helm
make dev-deploy

# Run dashboard dev server
make dev-dashboard
```

### Use the SDK

```typescript
import { XgenClient } from "@xgen-sandbox/sdk";

const client = new XgenClient({
  apiKey: "your-api-key",
  agentUrl: "http://localhost:8080",
});

const sandbox = await client.createSandbox({ template: "nodejs" });

// Execute a command
const result = await sandbox.exec("node -e \"console.log('Hello!')\"");
console.log(result.stdout); // "Hello!\n"

// Stream output
for await (const event of sandbox.execStream("npm install")) {
  if (event.type === "stdout") process.stdout.write(event.data);
}

// Interactive terminal
const terminal = await sandbox.openTerminal({ cols: 80, rows: 24 });
terminal.onData((data) => process.stdout.write(data));
terminal.write("ls -la\n");

await sandbox.destroy();
```

See [docs/sdk-guide.md](docs/sdk-guide.md) for all SDK languages.

## SDK Feature Matrix

| Feature | TypeScript | Python | Go | Rust |
|---------|:---------:|:------:|:--:|:----:|
| exec() | O | O | O | O |
| execStream() | O | O | O | - |
| openTerminal() | O | O | O | - |
| File operations | O | O | O | O |
| File watching | O | O | O | O |
| Port events | O | O | O | O |
| Auto-reconnect | O | O | O | - |

See [docs/sdk-feature-matrix.md](docs/sdk-feature-matrix.md) for full details.

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
├── dashboard/          # Next.js 15 — Admin UI (metrics, sandbox management, terminal)
├── deploy/
│   ├── helm/           # Helm chart
│   ├── dev/            # Kind cluster config
│   └── grafana/        # Grafana dashboard JSON
├── examples/           # SDK usage examples (8 examples across all languages)
├── docs/               # Documentation (10 guides)
├── docker-compose.yml  # Local development with Docker Compose
├── .env.example        # Environment variable template
└── Makefile            # Build automation (run `make help` for all targets)
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | System design, pod structure, protocol |
| [API Reference](docs/api-reference.md) | REST API endpoints and WebSocket protocol |
| [SDK Guide](docs/sdk-guide.md) | TypeScript, Python, Go, Rust SDK usage |
| [SDK Feature Matrix](docs/sdk-feature-matrix.md) | Feature comparison across all SDKs |
| [Deployment](docs/deployment.md) | Local dev, Helm chart, production config |
| [Security](docs/security.md) | Auth, RBAC, network policies, pod security |
| [Local Testing](docs/local-testing.md) | Step-by-step local development setup |
| [Performance](docs/performance.md) | Startup latency, resource overhead, scaling limits |
| [Troubleshooting](docs/troubleshooting.md) | Common issues and resolution steps |
| [Examples](docs/examples.md) | SDK usage example guide |

## Development

Run `make help` to see all available targets:

```bash
# Build
make build              # Build agent and sidecar binaries
make build-images       # Build all Docker images
make build-dashboard    # Build Next.js dashboard
make build-sdk          # Build TypeScript SDK

# Local development
make dev-cluster        # Create Kind cluster and load images
make dev-deploy         # Deploy to Kind cluster via Helm
make dev-dashboard      # Run dashboard dev server
make dev-agent          # Run agent with hot reload (requires air)
make dev-reload         # Rebuild images and restart in Kind
make dev-teardown       # Delete Kind cluster

# Testing & linting
make test               # Run Go tests (agent + sidecar)
make test-sdk           # Run TypeScript SDK tests
make lint               # Run go vet

# Utilities
make tidy               # Run go mod tidy
make help               # Show all targets with descriptions
```

Or use Docker Compose:

```bash
docker compose up -d      # Start agent + dashboard
docker compose logs -f    # Follow logs
docker compose down       # Stop all services
```

## CI/CD

| Workflow | Trigger | Description |
|----------|---------|-------------|
| [CI](.github/workflows/ci.yml) | Push / PR to main | Lint, test, dashboard type-check, SDK build, Docker build, Trivy security scan |
| [E2E](.github/workflows/e2e.yml) | Push / PR to main | Kind cluster deploy, full sandbox lifecycle test (create, exec, delete) |
| [Release](.github/workflows/release.yml) | Tag `v*` push | Build binaries, push versioned Docker images, create GitHub Release with changelog |

## License

MIT
