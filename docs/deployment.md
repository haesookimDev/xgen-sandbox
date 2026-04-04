# Deployment Guide

## Local Development

### Prerequisites

- Go 1.22+
- Docker
- [Kind](https://kind.sigs.k8s.io/) (Kubernetes in Docker)
- [Helm](https://helm.sh/) 3.x
- kubectl

### Step 1: Build

```bash
# Build Go binaries
make build

# Build Docker images
make build-images
```

This creates three images:
- `ghcr.io/xgen-sandbox/agent:latest`
- `ghcr.io/xgen-sandbox/sidecar:latest`
- `ghcr.io/xgen-sandbox/runtime-base:latest`

### Step 2: Create Kind Cluster

```bash
make dev-cluster
```

This creates a Kind cluster named `xgen-sandbox` with port mappings:
- `localhost:8080` → Agent HTTP API
- `localhost:8443` → Agent HTTPS (if configured)

### Step 3: Deploy

```bash
make dev-deploy
```

Deploys the Helm chart to the `xgen-system` namespace with local image pull policy.

### Step 4: Verify

```bash
kubectl get pods -n xgen-system
# NAME                         READY   STATUS    RESTARTS   AGE
# xgen-agent-xxxxx-xxxxx       1/1     Running   0          30s

# Test the API
curl http://localhost:8080/healthz
# ok
```

### Reload After Changes

```bash
# Rebuild images and restart
make dev-reload
```

### Teardown

```bash
make dev-teardown
```

---

## Helm Chart Configuration

### Install

```bash
helm upgrade --install xgen-sandbox deploy/helm/xgen-sandbox \
  --namespace xgen-system \
  --create-namespace
```

### Values Reference

#### Agent

```yaml
agent:
  image:
    repository: ghcr.io/xgen-sandbox/agent
    tag: latest
    pullPolicy: IfNotPresent
  replicas: 1
  service:
    type: ClusterIP    # or LoadBalancer, NodePort
    port: 8080
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
  env:
    SANDBOX_NAMESPACE: xgen-sandboxes
    PREVIEW_DOMAIN: preview.example.com
    API_KEY: <your-api-key>
    JWT_SECRET: <your-jwt-secret>
```

#### Sidecar & Runtime

```yaml
sidecar:
  image:
    repository: ghcr.io/xgen-sandbox/sidecar
    tag: latest

runtime:
  baseImage: ghcr.io/xgen-sandbox/runtime-base:latest
```

#### Sandbox Settings

```yaml
sandbox:
  namespace: xgen-sandboxes
  defaultTimeout: 1h       # Default sandbox lifetime
  maxTimeout: 24h           # Maximum allowed timeout
  warmPoolSize: 0           # Number of pre-created pods (0 = disabled)
  resourceQuota:
    pods: "50"              # Max pods in sandbox namespace
    requestsCpu: "25"       # Total CPU requests limit
    requestsMemory: "25Gi"  # Total memory requests limit
    limitsCpu: "50"         # Total CPU limits
    limitsMemory: "50Gi"    # Total memory limits
```

#### Ingress

```yaml
ingress:
  enabled: true
  className: traefik         # or nginx
  host: agent.example.com    # Agent API domain
  previewDomain: preview.example.com  # Preview wildcard domain
  tls: true
  clusterIssuer: letsencrypt-prod     # cert-manager issuer
```

When enabled, the Ingress routes:
- `agent.example.com` → Agent service
- `*.preview.example.com` → Agent service (for preview URL routing)

TLS certificates are managed by cert-manager. You need:
- A wildcard DNS record `*.preview.example.com` pointing to your ingress
- A cert-manager ClusterIssuer named `letsencrypt-prod`

#### Autoscaling

```yaml
autoscaling:
  enabled: true
  minReplicas: 1
  maxReplicas: 5
  targetCPUUtilization: 70
```

Creates a HorizontalPodAutoscaler that scales the agent deployment based on CPU usage.

#### Pod Disruption Budget

```yaml
podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

Ensures at least one agent pod is always available during voluntary disruptions (e.g., node drain).

---

## Environment Variables

All agent configuration is via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_LISTEN_ADDR` | `:8080` | HTTP listen address |
| `PREVIEW_DOMAIN` | `preview.localhost` | Base domain for preview URLs |
| `AGENT_EXTERNAL_URL` | `http://localhost:8080` | Public URL for WebSocket URLs in responses |
| `SANDBOX_NAMESPACE` | `xgen-sandboxes` | K8s namespace for sandbox pods |
| `SIDECAR_IMAGE` | `ghcr.io/xgen-sandbox/sidecar:latest` | Sidecar container image |
| `RUNTIME_BASE_IMAGE` | `ghcr.io/xgen-sandbox/runtime-base:latest` | Default runtime image |
| `DEFAULT_TIMEOUT` | `1h` | Default sandbox lifetime |
| `MAX_TIMEOUT` | `24h` | Maximum sandbox lifetime |
| `WARM_POOL_SIZE` | `0` | Pre-created pods per template |
| `API_KEY` | `xgen_dev_key` | API key (maps to admin role) |
| `JWT_SECRET` | (dev default) | HMAC-SHA256 signing key |

---

## Production Checklist

### Security

- [ ] Change `API_KEY` from default
- [ ] Set a strong random `JWT_SECRET` (at least 32 bytes)
- [ ] Enable TLS via Ingress
- [ ] Review and restrict `PREVIEW_DOMAIN`
- [ ] Set appropriate `resourceQuota` values

### Reliability

- [ ] Set `agent.replicas` to 2+ or enable autoscaling
- [ ] Enable `podDisruptionBudget`
- [ ] Set `warmPoolSize` > 0 for faster startup
- [ ] Configure appropriate `defaultTimeout` and `maxTimeout`

### Observability

- [ ] Scrape `/metrics` with Prometheus
- [ ] Set up Grafana dashboards for `xgen_*` metrics
- [ ] Forward structured logs (JSON) to your log aggregator
- [ ] Monitor `xgen_sandboxes_active` for capacity planning

### Networking

- [ ] Configure wildcard DNS for preview domain
- [ ] Set up cert-manager for TLS certificates
- [ ] Verify NetworkPolicy allows only expected traffic
- [ ] Set `AGENT_EXTERNAL_URL` to the public-facing URL

---

## Building Runtime Images

### Additional runtimes

Build and load additional runtime images:

```bash
# Node.js runtime
docker build -t ghcr.io/xgen-sandbox/runtime-nodejs:latest ./runtime/nodejs

# Python runtime
docker build -t ghcr.io/xgen-sandbox/runtime-python:latest ./runtime/python

# Go runtime
docker build -t ghcr.io/xgen-sandbox/runtime-go:latest ./runtime/go

# GUI runtime (VNC desktop)
docker build -t ghcr.io/xgen-sandbox/runtime-gui:latest ./runtime/gui
```

For Kind clusters, load images:

```bash
kind load docker-image ghcr.io/xgen-sandbox/runtime-nodejs:latest --name xgen-sandbox
```

### Custom Runtimes

Create a custom runtime by extending the base image:

```dockerfile
FROM ghcr.io/xgen-sandbox/runtime-base:latest

RUN apt-get update && apt-get install -y your-packages \
    && rm -rf /var/lib/apt/lists/*

USER sandbox
WORKDIR /home/sandbox/workspace
```

Then reference it in the sandbox creation request or configure the agent to map a template name to your image.

---

## Monitoring with Prometheus & Grafana

### Prometheus Scrape Config

```yaml
# prometheus.yml
scrape_configs:
  - job_name: 'xgen-agent'
    kubernetes_sd_configs:
      - role: pod
        namespaces:
          names: ['xgen-system']
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app]
        regex: xgen-agent
        action: keep
      - source_labels: [__meta_kubernetes_pod_container_port_name]
        regex: http
        action: keep
```

### Key Metrics to Monitor

| Metric | Alert Threshold | Description |
|--------|----------------|-------------|
| `xgen_sandboxes_active` | > 80% of quota pods | Approaching capacity |
| `xgen_http_request_duration_seconds{quantile="0.99"}` | > 5s | High API latency |
| `rate(xgen_http_requests_total{status=~"5.."}[5m])` | > 1/s | Server errors |
| `rate(xgen_sandbox_create_total[5m])` | — | Creation rate |
