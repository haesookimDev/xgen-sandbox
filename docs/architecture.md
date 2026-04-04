# Architecture

## Overview

xgen-sandbox is a Kubernetes-native platform that provides isolated code execution environments. The system consists of four main components:

1. **Agent** — Go HTTP server that manages sandbox lifecycle via the K8s API
2. **Sidecar** — Go process running inside each sandbox pod, handling exec/fs/port operations
3. **Runtime** — Container images providing the execution environment (base, nodejs, python, go, gui)
4. **SDKs & Browser** — Client libraries and React components for interacting with sandboxes

## System Diagram

```
                    ┌─────────────────────────────────────────────┐
                    │              Client Layer                    │
                    │  SDK (TS/Py/Go/Rust)  │  Browser Components │
                    └──────────┬──────────────────────┬───────────┘
                               │ REST + WS            │ Preview URL
                               ▼                      ▼
                    ┌──────────────────────────────────────────────┐
                    │              Agent (xgen-system namespace)    │
                    │                                              │
                    │  ┌──────────┐ ┌───────────┐ ┌────────────┐  │
                    │  │HTTP Server│ │ WS Proxy  │ │Preview     │  │
                    │  │(chi)     │ │(mux)      │ │Router      │  │
                    │  └────┬────��┘ └─────┬─────┘ └──────┬─────┘  │
                    │       │             │              │         │
                    │  ┌────┴─────────────┴─────���────────┴─────┐  │
                    │  │         Pod Manager + Warm Pool        │  │
                    │  └────────────────┬───���──────────────────┘  │
                    │                   │ K8s API                  │
                    └───────────────────┼───────��──────────────────┘
                                        │
                    ┌───────────────────┼──────────────────────────┐
                    │  xgen-sandboxes namespace                    │
                    │                   ▼                          │
                    │  ┌─────────────────────────────────┐        │
                    │  │         Sandbox Pod              │        │
                    │  │  ┌─────────┐ ┌──��─────┐ ┌─────┐ │        │
                    │  │  │ Sidecar �� │Runtime │ │ VNC │ │        │
                    │  │  │ :9000   │ │(sleep) │ │:6080│ │        │
                    │  │  └─────────┘ └────────┘ └─────┘ │        │
                    │  │       shared: /home/sandbox/workspace     │
                    │  └─────────────────────────────────┘        │
                    └─────────────────────────────────────────────┘
```

## Component Details

### Agent

The Agent is the control plane. It runs in the `xgen-system` namespace and provides:

- **REST API** — CRUD for sandboxes, synchronous exec, auth token exchange
- **WebSocket Proxy** — Multiplexed bidirectional proxy between clients and sidecar
- **Preview Router** — Dynamic reverse proxy routing `sbx-{id}-{port}.preview.{domain}` to pod IPs (supports both HTTP and WebSocket upgrade)
- **Pod Manager** — Creates/deletes K8s pods, watches for readiness, caches pod info
- **Warm Pool** — Maintains pre-created pods per template for fast (<1s) sandbox startup
- **Auth** — JWT token generation/validation with RBAC (admin/user/viewer)
- **Metrics** — Prometheus metrics at `/metrics`
- **Rate Limiting** — Per-client IP token bucket (120 req/min)
- **Audit Logging** — Structured logs for mutating API operations

### Sidecar

The Sidecar runs as a container in every sandbox pod. It communicates with the Agent over a binary WebSocket protocol:

- **Exec Manager** — Starts processes, streams stdin/stdout/stderr, handles signals and PTY resize
- **Filesystem Handler** — Read, write, list, remove files; watches for file changes
- **Port Detector** — Monitors for newly opened TCP ports and reports them
- **Health Check** — `/healthz` and `/readyz` on port 9001

### Sandbox Pod Structure

Each sandbox pod contains 2-3 containers sharing a process namespace:

| Container | Port | Purpose | Resources |
|-----------|------|---------|-----------|
| **sidecar** | 9000 (WS), 9001 (health) | Process/file/port management | 50m-200m CPU, 32-64Mi mem |
| **runtime** | — | User code execution (`sleep infinity`) | 250m-1000m CPU, 256-512Mi mem |
| **vnc** (if `gui=true`) | 6080 (noVNC) | Xvfb + x11vnc + websockify | 100m-500m CPU, 128-256Mi mem |

Shared volume: `emptyDir` (1Gi limit) mounted at `/home/sandbox/workspace`

### Runtime Images

| Image | Base | Additional Packages |
|-------|------|---------------------|
| `runtime-base` | Ubuntu 22.04 | curl, wget, git, build-essential, vim, jq |
| `runtime-nodejs` | runtime-base | Node.js 20, yarn, pnpm |
| `runtime-python` | runtime-base | Python 3.11, pip, venv |
| `runtime-go` | runtime-base | Go (latest) |
| `runtime-gui` | runtime-base | Xvfb, x11vnc, fluxbox, xterm, noVNC, websockify |

## Communication Flow

### Sandbox Creation

```
1. Client ─── POST /api/v1/sandboxes ──▶ Agent
2. Agent  ─── Check warm pool          ──▶ Claim or Create Pod
3. Agent  ─── Watch pod status          ──▶ K8s API
4. K8s    ─── Pod becomes Ready         ──▶ Agent (onReady callback)
5. Agent  ─── Connect WS to sidecar    ──▶ Sidecar :9000
6. Agent  ─── Return SandboxResponse   ──▶ Client
```

### Command Execution (REST)

```
1. Client ─── POST /api/v1/sandboxes/{id}/exec ──▶ Agent
2. Agent  ─��─ ExecSync via WS proxy             ──▶ Sidecar
3. Sidecar ── MsgExecStart → Run process         ──▶ Runtime
4. Sidecar ── MsgExecStdout/Stderr/Exit          ──▶ Agent
5. Agent  ─── ExecResponse                      ──▶ Client
```

### Interactive Terminal (WebSocket)

```
1. Client ─── WS /api/v1/sandboxes/{id}/ws ──▶ Agent
2. Agent  ─── Proxy WS frames               ──▶ Sidecar
3. Client ─── MsgExecStart (tty=true)        ──▶ Sidecar (via Agent)
4. Sidecar ── MsgExecStdout (terminal output)──▶ Client (via Agent)
5. Client ─── MsgExecStdin (keyboard input)  ──▶ Sidecar (via Agent)
6. Client ─── MsgExecResize (cols, rows)     ──▶ Sidecar (via Agent)
```

### Preview URL Routing

```
1. Browser ── GET https://sbx-{id}-3000.preview.example.com ──▶ Ingress
2. Ingress ── Route to Agent service                         ──▶ Agent
3. Agent   ── Parse subdomain → sandbox ID + port            ──▶ Preview Router
4. Router  ── Reverse proxy to pod_ip:3000                   ──▶ Sandbox Pod
```

For WebSocket traffic (including noVNC), the router detects the `Upgrade: websocket` header and switches to raw TCP tunneling.

## WebSocket Binary Protocol

All WebSocket messages use a binary envelope format:

```
┌──────────┬──────────┬──────────┬────────────────────┐
│ Type     │ Channel  │ ID       │ Payload (msgpack)  │
│ 1 byte   │ 4 bytes  │ 4 bytes  │ variable           │
│ (uint8)  │ (uint32) │ (uint32) │                    │
└──────────┴──────────┴──────────┴────────────────────┘
         9-byte header              body
```

- **Type** — Message type (see below)
- **Channel** — Logical channel for multiplexing (0 = control, 1+ = sessions)
- **ID** — Request/response correlation ID
- **Payload** — MessagePack-encoded data

### Message Types

| Code | Name | Direction | Payload |
|------|------|-----------|---------|
| `0x01` | Ping | Both | — |
| `0x02` | Pong | Both | — |
| `0x03` | Error | Server→Client | `{code, message}` |
| `0x04` | Ack | Server→Client | — |
| `0x20` | ExecStart | Client→Server | `{command, args, env, cwd, tty, cols, rows}` |
| `0x21` | ExecStdin | Client→Server | raw bytes |
| `0x22` | ExecStdout | Server→Client | raw bytes |
| `0x23` | ExecStderr | Server→Client | raw bytes |
| `0x24` | ExecExit | Server→Client | `{exit_code}` |
| `0x25` | ExecSignal | Client→Server | `{signal}` |
| `0x26` | ExecResize | Client→Server | `{cols, rows}` |
| `0x30` | FsRead | Client→Server | `{path}` |
| `0x31` | FsWrite | Client→Server | `{path, content, mode}` |
| `0x32` | FsList | Client→Server | `{path}` |
| `0x33` | FsRemove | Client→Server | `{path, recursive}` |
| `0x34` | FsWatch | Client→Server | `{path, unwatch}` |
| `0x35` | FsEvent | Server→Client | `{path, type}` |
| `0x40` | PortOpen | Server→Client | `{port}` |
| `0x41` | PortClose | Server→Client | `{port}` |
| `0x50` | SandboxReady | Server→Client | — |
| `0x51` | SandboxError | Server→Client | `{code, message}` |
| `0x52` | SandboxStats | Server→Client | `{cpu_percent, memory_bytes, disk_used_bytes}` |

## Warm Pool

The warm pool pre-creates sandbox pods so new sandboxes can start in under 1 second instead of waiting for container pull and initialization.

```
Startup:
  Pool fills with N pods per template (configurable via WARM_POOL_SIZE)

On CreateSandbox:
  1. Try Claim(template) → get warm pod ID
  2. If claimed: RemapPod(warmID → sandboxID), set running immediately
  3. Async: Replenish(template) → create replacement pod
  4. If no warm pod: create new pod normally

Pod naming: warm-{uuid} → remapped to sbx-{sandboxID}
```

## Kubernetes Resources

The Helm chart creates:

| Resource | Namespace | Purpose |
|----------|-----------|---------|
| Namespace `xgen-system` | — | Agent components |
| Namespace `xgen-sandboxes` | — | Sandbox pods |
| Deployment `xgen-agent` | xgen-system | Agent server |
| Service `xgen-agent` | xgen-system | Agent ClusterIP |
| ServiceAccount `xgen-agent` | xgen-system | K8s API access |
| ClusterRole `xgen-agent` | — | Pod CRUD permissions |
| NetworkPolicy `sandbox-isolation` | xgen-sandboxes | Ingress/egress rules |
| ResourceQuota | xgen-sandboxes | Pod/CPU/memory limits |
| Ingress (optional) | xgen-system | External access |
| HPA (optional) | xgen-system | Auto-scaling |
| PDB (optional) | xgen-system | Disruption budget |
