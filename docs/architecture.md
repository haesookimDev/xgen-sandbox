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

- **Exec Manager** — Starts processes via `nsenter --mount` into the runtime container's mount namespace, streams stdin/stdout/stderr, handles signals and PTY resize. Requires `CAP_SYS_ADMIN` (sidecar runs as root with all other caps dropped).
- **Filesystem Handler** — Read, write, list, remove files; watches for file changes
- **Port Detector** — Monitors `/proc/net/tcp` for newly opened TCP ports and reports them (works across containers because the pod shares a network namespace)
- **Health Check** — `/healthz` and `/readyz` on port 9001

### Sandbox Pod Structure

Each sandbox pod contains 2-3 containers sharing a process namespace:

| Container | Port | Purpose | Resources |
|-----------|------|---------|-----------|
| **sidecar** | 9000 (WS), 9001 (health) | Process/file/port management (runs as root, caps: SYS_ADMIN + SYS_PTRACE only) | 50m-200m CPU, 32-64Mi mem |
| **runtime** | — | User code execution (`sleep infinity`, runs as UID 1000) | 250m-1000m CPU, 256-512Mi mem |
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

#### Capability Variants

When the `sudo` or `browser` capability is requested, a variant image is used:

| Image | Base | Additional Packages |
|-------|------|---------------------|
| `runtime-base-sudo` | runtime-base | sudo (passwordless for sandbox user) |
| `runtime-nodejs-sudo` | runtime-nodejs | sudo |
| `runtime-python-sudo` | runtime-python | sudo |
| `runtime-go-sudo` | runtime-go | sudo |
| `runtime-gui-browser` | runtime-gui | Chromium, fonts-liberation, libnss3, sudo |

The `-sudo` variants add `SETUID`/`SETGID` Linux capabilities and `allowPrivilegeEscalation: true` to the runtime container security context. The `browser` capability uses `runtime-gui-browser` which includes Chromium and implies `gui: true`.

## Communication Flow

### Sandbox Creation

```
1. Client ─── POST /api/v1/sandboxes ──▶ Agent
2. Agent  ─── Validate capabilities     ──▶ Select image + security context
3. Agent  ─── Check warm pool*          ──▶ Claim or Create Pod
4. Agent  ─── Create per-pod NetworkPolicy (if git-ssh) ──▶ K8s API
5. K8s    ─── Pod becomes Ready         ──▶ Agent (onReady callback)
6. Agent  ─── Connect WS to sidecar    ──▶ Sidecar :9000
7. Agent  ─── Return SandboxResponse   ──▶ Client
```

\* Warm pool is only used for sandboxes without capabilities. Capability-enabled sandboxes always create a fresh pod.

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

## Command Execution Flow (nsenter)

The sidecar executes commands inside the runtime container using Linux namespace entry:

```
1. SDK sends ExecStart { command: "sh", args: ["-c", "node server.js"] }
2. Agent WS proxy forwards binary message to sidecar
3. Sidecar findRuntimePID() scans /proc for "sleep infinity" process
4. Sidecar runs: nsenter --target <PID> --mount --wd /home/sandbox/workspace -- sh -c "node server.js"
5. nsenter enters the runtime container's mount namespace (CAP_SYS_ADMIN required)
6. Command executes with runtime's filesystem (node, python, etc. available)
7. stdout/stderr streamed back via WS binary protocol
```

Key points:
- `--mount` enters the runtime container's filesystem namespace (to access node/python binaries)
- `--pid` is NOT needed because `shareProcessNamespace: true` already shares PIDs
- `--wd` sets the working directory (nsenter built-in, no shell wrapper needed)
- The sidecar runs as root (UID 0) with only `CAP_SYS_ADMIN` + `CAP_SYS_PTRACE` capabilities
- The runtime container runs as non-root (UID 1000) with no special capabilities
- Network namespace is shared at the pod level, so ports opened by the runtime are visible to the port detector

## Debugging

Use the debug script for diagnosing issues:

```bash
# Overview of all components
./scripts/debug-sandbox.sh

# Debug a specific sandbox
./scripts/debug-sandbox.sh <sandbox-id>

# Test exec via REST API
./scripts/debug-sandbox.sh exec <sandbox-id> echo hello

# Check sidecar logs
kubectl logs -n xgen-sandboxes sbx-<id> -c sidecar

# Check runtime container logs
kubectl logs -n xgen-sandboxes sbx-<id> -c runtime

# Check agent logs
kubectl logs -n xgen-system deployment/xgen-agent --tail=50

# Verify sidecar capabilities
kubectl exec -n xgen-sandboxes sbx-<id> -c sidecar -- cat /proc/1/status | grep Cap
```

### Common Issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| `nsenter: Operation not permitted` | Missing `CAP_SYS_ADMIN` | Sidecar must run as root with SYS_ADMIN capability |
| `sandbox service unavailable` (502) | No process listening on the requested port | Check exec output for errors; verify server binds to `0.0.0.0` |
| `sandbox not ready` (503) | Pod IP not yet assigned | Wait for pod readiness; check pod status |
| Empty exec output | SDK not handling MsgError | Check sidecar logs for `exec start failed` |
| `runtime container process not found` | Cannot find `sleep infinity` in /proc | Verify runtime container is running with `sleep infinity` command |
