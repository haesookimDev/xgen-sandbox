# Performance Characteristics

## Sandbox Startup Latency

| Scenario | Expected Latency |
|----------|-----------------|
| Cold start (no warm pool) | 15-30s (image pull + pod scheduling + sidecar ready) |
| Warm pool hit | < 1s (pod already running, just remap) |
| Warm pool miss, image cached | 5-10s (pod scheduling + container start) |

## Exec Latency

| Operation | Expected Latency |
|-----------|-----------------|
| REST exec (simple command) | 100-500ms (WebSocket connect + exec + response) |
| WebSocket exec (connected) | 10-50ms (message round-trip) |
| File read (small file) | 10-30ms |
| File write (small file) | 10-30ms |

## Resource Overhead Per Sandbox

| Component | CPU Request | Memory Request |
|-----------|------------|----------------|
| Sidecar | 50m | 32Mi |
| Runtime (base) | 250m | 256Mi |
| Runtime (GUI/VNC) | 350m | 384Mi |
| **Total (base)** | **300m** | **288Mi** |

## Scaling Limits

| Metric | Default Limit | Configurable |
|--------|--------------|-------------|
| Max sandboxes per namespace | 50 pods (ResourceQuota) | Yes |
| Max concurrent WebSocket connections | Unlimited (per agent) | No |
| Rate limit | 120 req/min per client | `RATE_LIMIT_PER_MINUTE` |
| Warm pool size | 0 (disabled) | `WARM_POOL_SIZE`, `WARM_POOL_SIZES` |
| Agent replicas | 1-5 (HPA) | `autoscaling.*` |

## File Watcher

- Polling interval: 1 second
- Latency for detecting changes: 1-2 seconds
- Overhead scales with number of watched files

## Recommendations

- **Enable warm pool** for production: `WARM_POOL_SIZE=3` eliminates cold start latency
- **Use template-specific pools**: `WARM_POOL_SIZES=base:3,nodejs:2,gui:1`
- **Set resource limits** via API `resources` field to prevent noisy neighbors
- **Monitor** `xgen_sandbox_pod_create_duration_seconds` for startup latency trends
