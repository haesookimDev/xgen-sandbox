# Session Registry

`xgen` keeps a local JSON registry at `~/.xgen/sessions.json` by default.
Set `XGEN_SESSION_REGISTRY` to override the path.

Each record stores:

- `session_id`
- `sandbox_id`
- `template`
- `cwd`
- `ports`
- `capabilities`
- `created_at_ms`
- `expires_at_ms`
- `last_used_at_ms`
- `metadata`

The CLI also writes `xgen_session_id`, `xgen_session_registry=cli`, and
`xgen_cwd` into sandbox metadata when creating a sandbox.

## Policy

The CLI uses the same millisecond-based policy knobs intended for SDKs:

- `XGEN_SESSION_IDLE_TTL_MS` controls how long an unused local session remains
  eligible for automatic keepalive. Default: `1800000` (30 minutes).
- `XGEN_SESSION_KEEPALIVE_AFTER_MS` controls when a session is automatically
  kept alive before server-side expiry. Default: `300000` (5 minutes).

Before `exec`, `fs`, and `port wait`, `xgen` checks the local registry. If the
session is within the keepalive window, it calls the agent keepalive endpoint
and refreshes registry timestamps from the agent. If the session exceeded idle
TTL, the command fails with `SESSION_IDLE_EXPIRED`.

`xgen session gc --json` removes local records for expired, idle, and missing
sandboxes. By default it also destroys tracked expired/idle sandboxes; pass
`--destroy=false` to only update the local registry.
