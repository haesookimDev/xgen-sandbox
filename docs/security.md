# Security

## Overview

xgen-sandbox runs untrusted code in isolated Kubernetes pods. Security is enforced at multiple layers:

1. **Authentication** — API key + JWT token
2. **Authorization** — Role-based access control (RBAC)
3. **Pod Isolation** — Security contexts, seccomp, capability drop
4. **Network Isolation** — Kubernetes NetworkPolicy
5. **Resource Isolation** — ResourceQuota and per-pod limits
6. **Lifecycle Control** — Automatic timeout and cleanup

---

## Authentication

### API Keys

API keys are the primary credentials. Each key is mapped to a role (admin, user, or viewer). The key is passed via:

```
Authorization: ApiKey <your-api-key>
```

### JWT Tokens

For better security, exchange an API key for a short-lived JWT token:

```bash
curl -X POST http://localhost:8080/api/v1/auth/token \
  -H "Content-Type: application/json" \
  -d '{"api_key": "your-key"}'
```

Response:

```json
{
  "token": "eyJ...",
  "expires_at": "2024-01-15T12:15:00Z"
}
```

Tokens expire after **15 minutes** and are signed with HMAC-SHA256. Use them via:

```
Authorization: Bearer <jwt-token>
```

SDKs handle token exchange and refresh automatically.

### WebSocket Authentication

WebSocket connections pass the token as a query parameter:

```
ws://agent/api/v1/sandboxes/{id}/ws?token=<jwt-token>
```

---

## Role-Based Access Control (RBAC)

### Roles

| Role | Description |
|------|-------------|
| **admin** | Full access to all operations |
| **user** | Create, read, write, exec, files (no delete) |
| **viewer** | Read-only access |

### Permissions

| Permission | admin | user | viewer |
|------------|-------|------|--------|
| `sandbox:create` | yes | yes | — |
| `sandbox:read` | yes | yes | yes |
| `sandbox:write` | yes | yes | — |
| `sandbox:delete` | yes | — | — |
| `sandbox:exec` | yes | yes | — |
| `sandbox:files` | yes | yes | — |

### Route-Permission Mapping

| Endpoint | Permission |
|----------|------------|
| `POST /api/v1/sandboxes` | `sandbox:create` |
| `GET /api/v1/sandboxes` | `sandbox:read` |
| `GET /api/v1/sandboxes/{id}` | `sandbox:read` |
| `DELETE /api/v1/sandboxes/{id}` | `sandbox:delete` |
| `POST /api/v1/sandboxes/{id}/keepalive` | `sandbox:write` |
| `POST /api/v1/sandboxes/{id}/exec` | `sandbox:exec` |
| `GET /api/v1/sandboxes/{id}/ws` | `sandbox:exec` |
| `GET /api/v1/sandboxes/{id}/services` | `sandbox:read` |

---

## Pod Security

### Pod-Level Security Context

Every sandbox pod is created with:

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000       # "sandbox" user
  runAsGroup: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault
```

- **runAsNonRoot** — Prevents any container from running as root
- **runAsUser/Group** — All processes run as the unprivileged `sandbox` user (UID 1000)
- **seccompProfile** — Applies the container runtime's default seccomp profile, blocking dangerous syscalls

### Container-Level Security Context

**Sidecar container:**

```yaml
securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop: ["ALL"]
```

**Runtime container (default):**

```yaml
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
```

**Runtime container (with `sudo` or `browser` capability):**

```yaml
securityContext:
  allowPrivilegeEscalation: true
  capabilities:
    drop: ["ALL"]
    add: ["SETUID", "SETGID"]
```

When the `sudo` or `browser` capability is requested, the runtime container uses a `-sudo` image variant with passwordless sudo configured. The `SETUID`/`SETGID` capabilities and `allowPrivilegeEscalation: true` are required for `sudo` to function. All other capabilities remain dropped, and the pod-level seccomp profile still applies.

Root filesystem is writable for the runtime container because user code may need to install packages.

**VNC container (when GUI enabled):**

```yaml
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop: ["ALL"]
```

### Additional Pod Settings

- `automountServiceAccountToken: false` — No K8s API access from sandbox pods
- `restartPolicy: Never` — Pods are ephemeral; they don't restart
- `shareProcessNamespace: true` — Sidecar can manage processes in the runtime container

---

## Network Isolation

### NetworkPolicy

Sandbox pods are governed by a NetworkPolicy in the `xgen-sandboxes` namespace:

**Ingress (inbound traffic):**

| Source | Ports | Purpose |
|--------|-------|---------|
| `xgen-system` namespace | 9000/TCP | Sidecar WebSocket |
| `xgen-system` namespace | 6080/TCP | VNC (noVNC) |
| `xgen-system` namespace | 1024-65535/TCP | Preview URL proxying |

Only the Agent (running in `xgen-system`) can reach sandbox pods. No inter-sandbox communication is allowed.

**Egress (outbound traffic):**

| Destination | Ports | Purpose |
|-------------|-------|---------|
| Public internet (excludes private IPs) | 80, 443/TCP | Package installation, API calls |
| All namespaces | 53/UDP, 53/TCP | DNS resolution |

Blocked outbound destinations:
- `10.0.0.0/8` (internal cluster network)
- `172.16.0.0/12` (internal cluster network)
- `192.168.0.0/16` (internal cluster network)

This prevents sandboxes from scanning or attacking internal cluster services while allowing normal internet access.

**Per-pod NetworkPolicy (`git-ssh` capability):**

When the `git-ssh` capability is requested, the Agent creates an additional NetworkPolicy scoped to the specific sandbox pod (via `xgen.io/sandbox-id` label selector):

| Destination | Ports | Purpose |
|-------------|-------|---------|
| Public internet (excludes private IPs) | 22/TCP | SSH-based git operations |

This policy is additive — the pod gets the union of the base policy (80/443/53) and the git-ssh policy (22). The per-pod NetworkPolicy is automatically deleted when the sandbox is destroyed.

---

## Resource Isolation

### Per-Pod Limits

Each sandbox pod has resource requests and limits:

| Container | CPU Request | CPU Limit | Memory Request | Memory Limit |
|-----------|-------------|-----------|----------------|--------------|
| Sidecar | 50m | 200m | 32Mi | 64Mi |
| Runtime | 250m | 1000m | 256Mi | 512Mi |
| VNC | 100m | 500m | 128Mi | 256Mi |

Total per pod: ~400m-1700m CPU, ~416Mi-832Mi memory

### Workspace Disk

The shared workspace volume is an `emptyDir` with a 1Gi size limit:

```yaml
volumes:
  - name: workspace
    emptyDir:
      sizeLimit: 1Gi
```

### Namespace Quota

The `xgen-sandboxes` namespace has a ResourceQuota (configurable via Helm values):

| Resource | Default Limit |
|----------|---------------|
| Pods | 50 |
| CPU requests | 25 cores |
| CPU limits | 50 cores |
| Memory requests | 25Gi |
| Memory limits | 50Gi |

---

## Lifecycle & Timeout

### Automatic Expiry

- **Default timeout:** 1 hour (configurable via `DEFAULT_TIMEOUT`)
- **Maximum timeout:** 24 hours (configurable via `MAX_TIMEOUT`)
- **Expiry check interval:** Every 10 seconds

When a sandbox expires:
1. Status set to `stopping`
2. `DeletePod` called with 10-second grace period
3. WebSocket disconnected
4. Sandbox removed from memory

### Force Delete

If a pod fails to terminate within 30 seconds after the initial delete:
- `ForceDeletePod` is called with `GracePeriodSeconds: 0`
- This immediately kills the pod without waiting for graceful shutdown

### Keep Alive

Clients can extend the timeout by calling:

```
POST /api/v1/sandboxes/{id}/keepalive
```

This extends the expiry by the default timeout duration (1 hour).

---

## Rate Limiting

API endpoints are protected by a per-client-IP token bucket rate limiter:

- **Limit:** 120 requests per minute per client IP
- **Response when exceeded:** `429 Too Many Requests`
- **Header used for client identification:** `X-Forwarded-For` (falls back to `RemoteAddr`)

Rate limiting applies to authenticated API routes only. Health check and metrics endpoints are not rate limited.

---

## Audit Logging

All mutating API operations (POST, DELETE) are logged with:

- **action** — HTTP method + path
- **subject** — Authenticated user (from JWT claims)
- **role** — User's RBAC role
- **status** — HTTP response status code
- **remote** — Client IP address

Example log entry (JSON):

```json
{
  "time": "2024-01-15T12:00:00Z",
  "level": "INFO",
  "msg": "audit",
  "action": "POST /api/v1/sandboxes",
  "subject": "api-key-hash",
  "role": "admin",
  "status": 201,
  "remote": "10.0.0.1:54321"
}
```

---

## Security Recommendations

### For Production Deployments

1. **Rotate API keys** regularly and use unique keys per user/service
2. **Use strong JWT secrets** — at least 32 random bytes, stored in K8s Secrets
3. **Enable TLS** via Ingress with cert-manager
4. **Restrict preview domain** — use a separate domain from your main application
5. **Monitor audit logs** for unusual patterns (mass creation, privilege escalation attempts)
6. **Set conservative resource quotas** based on your cluster capacity
7. **Keep images updated** — rebuild runtime images regularly for security patches
8. **Review NetworkPolicy** — adjust egress rules if sandboxes don't need internet access

### Known Limitations

- **No per-user sandbox isolation** — Any authenticated user can access any sandbox. Owner-based isolation would require adding a `user_id` field to sandboxes and checking it in handlers.
- **No container image scanning** — Runtime images are not automatically scanned for vulnerabilities.
- **Single API key** — The current implementation supports a single API key. Multi-key support with per-key roles would require a database backend.
- **In-memory state** — Sandbox state is stored in memory. Agent restarts lose track of running sandboxes (though pods persist in K8s and can be recovered).
