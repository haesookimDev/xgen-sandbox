# Troubleshooting Guide

## Common Issues

### Sandbox stuck in "starting" state

**Symptoms:** Sandbox status remains `starting` for more than 60 seconds.

**Possible causes:**
1. **Image pull failure** — Runtime image not available in the cluster.
2. **Insufficient resources** — Node doesn't have enough CPU/memory.
3. **Sidecar not ready** — Sidecar container failing readiness probe.

**Resolution:**
```bash
# Check pod status
kubectl get pods -n xgen-sandboxes -l xgen.io/sandbox-id=<SANDBOX_ID>

# Check pod events
kubectl describe pod sbx-<SANDBOX_ID> -n xgen-sandboxes

# Check sidecar logs
kubectl logs sbx-<SANDBOX_ID> -c sidecar -n xgen-sandboxes
```

### Sandbox exec returns connection error

**Symptoms:** `exec` calls fail with "no sidecar connection" or timeout.

**Possible causes:**
1. **Pod IP not reachable** — Network policy blocking agent-to-sandbox traffic.
2. **Sidecar crashed** — Check sidecar container status.

**Resolution:**
```bash
# Verify sidecar is running
kubectl get pod sbx-<SANDBOX_ID> -n xgen-sandboxes -o jsonpath='{.status.containerStatuses[?(@.name=="sidecar")].ready}'

# Test connectivity from agent
kubectl exec -n xgen-system deploy/xgen-agent -- wget -qO- http://<POD_IP>:9001/readyz
```

### Warm pool not filling

**Symptoms:** Warm pool shows 0 available pods despite `WARM_POOL_SIZE > 0`.

**Possible causes:**
1. **ResourceQuota exhausted** — Check namespace quotas.
2. **Image not available** — Runtime image not loaded in cluster.

**Resolution:**
```bash
kubectl get resourcequota -n xgen-sandboxes
kubectl get events -n xgen-sandboxes --sort-by='.lastTimestamp' | tail -20
```

### Agent crashes on startup

**Symptoms:** Agent pod in CrashLoopBackOff.

**Possible causes:**
1. **Missing secrets** — `API_KEY` or `JWT_SECRET` not set (required since v0.2).
2. **K8s API unreachable** — ServiceAccount permissions issue.

**Resolution:**
```bash
kubectl logs -n xgen-system deploy/xgen-agent --previous
kubectl get secret xgen-agent-secrets -n xgen-system
```

### Dashboard shows "Unauthorized"

**Symptoms:** All API calls return 401 after login.

**Possible causes:**
1. **JWT expired** — Default expiry is 15 minutes.
2. **Clock skew** — Agent and client have different system times.

**Resolution:** Log out and log in again with a valid API key.

### WebSocket disconnects frequently

**Symptoms:** Terminal or streaming connections drop unexpectedly.

**Possible causes:**
1. **Load balancer timeout** — Default idle timeouts (e.g., 60s on ALB).
2. **Rate limiting** — Too many concurrent connections.

**Resolution:**
- Configure load balancer idle timeout to 300s+
- Check `RATE_LIMIT_PER_MINUTE` setting
- SDKs automatically reconnect (up to 5 attempts with exponential backoff)
