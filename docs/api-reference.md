# API Reference

Base URL: `http://localhost:8080` (development)

## Authentication

All API endpoints (except `/healthz`, `/metrics`, and `/api/v1/auth/token`) require authentication.

### Supported Schemes

- **Bearer Token** — `Authorization: Bearer <jwt-token>`
- **API Key** — `Authorization: ApiKey <api-key>`

### Token Exchange

```
POST /api/v1/auth/token
```

Exchange an API key for a short-lived JWT token.

**Request:**

```json
{
  "api_key": "xgen_dev_key"
}
```

**Response:**

```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_at": "2024-01-15T12:15:00Z"
}
```

Tokens expire after 15 minutes. SDKs handle token refresh automatically.

---

## Sandbox Operations

### Create Sandbox

```
POST /api/v1/sandboxes
```

**Permission:** `sandbox:create`

**Request:**

```json
{
  "template": "nodejs",
  "timeout_seconds": 3600,
  "env": {
    "NODE_ENV": "development"
  },
  "ports": [3000, 8080],
  "gui": false,
  "capabilities": ["sudo", "git-ssh"],
  "metadata": {
    "user": "alice",
    "project": "demo"
  }
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `template` | string | `"base"` | Runtime template: `base`, `nodejs`, `python`, `go`, `gui` |
| `timeout_seconds` | int | 3600 | Sandbox lifetime in seconds (max: 86400) |
| `resources` | object | — | Resource limits: `{cpu, memory, disk}` |
| `env` | object | — | Environment variables for the runtime container |
| `ports` | int[] | — | Ports to expose via preview URLs |
| `gui` | bool | `false` | Enable VNC desktop (adds VNC container) |
| `capabilities` | string[] | — | Runtime capabilities (see below) |
| `metadata` | object | — | Arbitrary key-value metadata |

#### Capabilities

Capabilities are opt-in features that extend the sandbox runtime. Pass them as a string array:

| Capability | Description |
|------------|-------------|
| `sudo` | Enables `sudo` inside the sandbox. Uses a `-sudo` image variant with passwordless sudo configured for the `sandbox` user. Adds `SETUID`/`SETGID` Linux capabilities to the container. |
| `git-ssh` | Allows outbound SSH connections (port 22) for `git clone` over SSH. Creates a per-pod NetworkPolicy. Configure SSH keys via `exec` after creation. |
| `browser` | Installs Chromium browser in the sandbox. Implies `gui: true` and `sudo`. Automatically sets minimum resources to 2 CPU / 2Gi memory. |

**Response:** `201 Created`

```json
{
  "id": "a1b2c3d4",
  "status": "starting",
  "template": "nodejs",
  "ws_url": "http://localhost:8080/api/v1/sandboxes/a1b2c3d4/ws",
  "preview_urls": {
    "3000": "https://sbx-a1b2c3d4-3000.preview.example.com",
    "8080": "https://sbx-a1b2c3d4-8080.preview.example.com"
  },
  "vnc_url": null,
  "created_at": "2024-01-15T12:00:00Z",
  "expires_at": "2024-01-15T13:00:00Z",
  "capabilities": ["sudo", "git-ssh"],
  "metadata": {
    "user": "alice",
    "project": "demo"
  }
}
```

When `gui: true` (or `browser` capability), the response includes a `vnc_url`:

```json
{
  "vnc_url": "https://sbx-a1b2c3d4-6080.preview.example.com"
}
```

### List Sandboxes

```
GET /api/v1/sandboxes
```

**Permission:** `sandbox:read`

**Response:** `200 OK`

```json
[
  {
    "id": "a1b2c3d4",
    "status": "running",
    "template": "nodejs",
    "ws_url": "...",
    "preview_urls": { "3000": "..." },
    "created_at": "...",
    "expires_at": "..."
  }
]
```

### Get Sandbox

```
GET /api/v1/sandboxes/{id}
```

**Permission:** `sandbox:read`

**Response:** `200 OK` — Same shape as `SandboxResponse` above.

### Delete Sandbox

```
DELETE /api/v1/sandboxes/{id}
```

**Permission:** `sandbox:delete`

**Response:** `204 No Content`

### Keep Alive

Extend the sandbox timeout by the default duration (1 hour).

```
POST /api/v1/sandboxes/{id}/keepalive
```

**Permission:** `sandbox:write`

**Response:** `204 No Content`

---

## Execution

### Synchronous Exec

Execute a command and wait for it to complete.

```
POST /api/v1/sandboxes/{id}/exec
```

**Permission:** `sandbox:exec`

**Request:**

```json
{
  "command": "node",
  "args": ["-e", "console.log('hello')"],
  "env": { "FOO": "bar" },
  "cwd": "/home/sandbox/workspace",
  "timeout_seconds": 30
}
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `command` | string | **required** | Command to execute |
| `args` | string[] | `[]` | Command arguments |
| `env` | object | — | Additional environment variables |
| `cwd` | string | — | Working directory |
| `timeout_seconds` | int | 30 | Execution timeout |

**Response:** `200 OK`

```json
{
  "exit_code": 0,
  "stdout": "hello\n",
  "stderr": ""
}
```

---

## WebSocket

### Connect

```
GET /api/v1/sandboxes/{id}/ws?token={jwt-token}
```

**Permission:** `sandbox:exec`

Establishes a multiplexed binary WebSocket connection. See [architecture.md](architecture.md#websocket-binary-protocol) for the full protocol specification.

---

## Services

### List Services

List ports that are currently open in the sandbox.

```
GET /api/v1/sandboxes/{id}/services
```

**Permission:** `sandbox:read`

**Response:** `200 OK`

```json
[
  {
    "port": 3000,
    "preview_url": "https://sbx-a1b2c3d4-3000.preview.example.com"
  }
]
```

---

## Observability

### Health Check

```
GET /healthz
```

No authentication required. Returns `200 OK` with body `ok`.

### Prometheus Metrics

```
GET /metrics
```

No authentication required. Returns Prometheus text format.

**Available Metrics:**

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `xgen_http_requests_total` | Counter | method, path, status | Total HTTP requests |
| `xgen_http_request_duration_seconds` | Histogram | method, path | Request latency |
| `xgen_sandboxes_active` | Gauge | — | Currently active sandboxes |
| `xgen_sandbox_create_total` | Counter | — | Total sandboxes created |
| `xgen_sandbox_delete_total` | Counter | — | Total sandboxes deleted |

---

## Error Responses

All errors follow this format:

```json
{
  "error": "human-readable error message",
  "code": "ERROR_CODE",
  "details": "optional details"
}
```

| Status | Meaning |
|--------|---------|
| `400` | Invalid request body or parameters |
| `401` | Missing or invalid authentication |
| `403` | Insufficient permissions |
| `404` | Sandbox not found |
| `429` | Rate limit exceeded (120 req/min per client) |
| `500` | Internal server error |
| `502` | Sandbox service unavailable (proxy error) |

---

## Sandbox Status Lifecycle

```
starting ──▶ running ──▶ stopping ──▶ stopped
                │
                └──▶ error
```

| Status | Description |
|--------|-------------|
| `starting` | Pod is being created and containers are initializing |
| `running` | All containers are ready; exec and WS available |
| `stopping` | Deletion requested; grace period in progress |
| `stopped` | Pod deleted; sandbox removed |
| `error` | Pod failed to start or encountered a fatal error |
