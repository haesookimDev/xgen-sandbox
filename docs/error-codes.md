# API Error Codes

Stable, machine-readable identifiers returned by the xgen-sandbox agent.
They are the source of truth for both human operators and LLM/agent callers.

**Source of truth:** [`agent/api/v2/errors.go`](../agent/api/v2/errors.go). A
code is considered *registered* only if it appears both here and in that file.
The `TestRegistryMatchesDocs` test in the same package enforces the match.

## Response shapes

### v2 (`/api/v2/*`)

```json
{
  "code": "INVALID_PARAMETER",
  "message": "invalid port: 70000 (must be 1-65535)",
  "details": { "field": "ports", "value": 70000, "min": 1, "max": 65535 },
  "request_id": "xgen-1234",
  "retryable": false
}
```

### v1 (`/api/v1/*`, legacy)

v1 keeps its original shape. The same `code` is carried in the `code` field
and structured `details` are serialised as a compact JSON string.

```json
{
  "error": "invalid port: 70000 (must be 1-65535)",
  "code": "INVALID_PARAMETER",
  "details": "{\"field\":\"ports\",\"value\":70000,\"min\":1,\"max\":65535}"
}
```

## Codes

| Code | HTTP | Retryable | Summary | Typical `details` |
|---|---|---|---|---|
| `INVALID_REQUEST` | 400 | no | Request body could not be parsed | — |
| `INVALID_PARAMETER` | 400 | no | A parameter failed validation | `field`, `value`, `allowed` \| `min` \| `max` |
| `UNAUTHORIZED` | 401 | no | Missing or invalid credentials | — |
| `FORBIDDEN` | 403 | no | Authenticated but lacks permission | `permission` |
| `SANDBOX_NOT_FOUND` | 404 | no | Sandbox id does not exist | `sandbox_id` |
| `SANDBOX_NOT_READY` | 409 | **yes** | Sandbox has no pod IP yet | `sandbox_id` |
| `SANDBOX_EXPIRED` | 410 | no | Sandbox past expiry and removed | `sandbox_id`, `expired_at_ms` |
| `QUOTA_EXCEEDED` | 429 | **yes** | Rate limit or quota hit | `retry_after_ms` |
| `POD_CREATE_FAILED` | 503 | **yes** | Kubernetes pod creation failed | `reason`, `template` |
| `SIDECAR_UNREACHABLE` | 503 | **yes** | Sidecar WebSocket unreachable | `sandbox_id` |
| `EXEC_TIMEOUT` | 504 | no | Exec did not produce exit within timeout | `sandbox_id`, `timeout_ms` |
| `INTERNAL` | 500 | **yes** | Unclassified internal error | `reason` (optional) |

## Guidance for callers

- **Route by `code`, not `message`.** Messages are free-form and may change.
- **`retryable: true` is a hint, not a guarantee.** Back off exponentially and
  cap attempts. `details.retry_after_ms` (when present) is the server's preferred
  lower bound.
- **Log `request_id`** when surfacing errors — it correlates with agent logs.
- **LLM/agent callers:** decide next action from the code alone:
  - `INVALID_*` → surface to the user, don't retry.
  - `UNAUTHORIZED` / `FORBIDDEN` → stop; obtain fresh credentials.
  - `SANDBOX_NOT_READY` → retry with backoff.
  - `SANDBOX_NOT_FOUND` / `SANDBOX_EXPIRED` → create a fresh sandbox.
  - `QUOTA_EXCEEDED` → back off, ideally honoring `retry_after_ms`.
  - `POD_CREATE_FAILED` / `SIDECAR_UNREACHABLE` / `INTERNAL` → retry with
    exponential backoff; escalate after a few failures.
  - `EXEC_TIMEOUT` → raise the timeout or switch to streaming exec.

## Adding a new code

1. Add an `ErrorCode` constant and a `registry` entry in
   [`agent/api/v2/errors.go`](../agent/api/v2/errors.go).
2. Add a row to the **Codes** table above.
3. `go test ./agent/api/v2/...` — the registry/docs sync test must pass.
