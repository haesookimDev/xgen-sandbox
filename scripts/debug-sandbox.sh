#!/usr/bin/env bash
# debug-sandbox.sh — Diagnose sandbox issues
#
# Usage:
#   ./scripts/debug-sandbox.sh                  # Show all sandboxes
#   ./scripts/debug-sandbox.sh <sandbox-id>     # Debug a specific sandbox
#   ./scripts/debug-sandbox.sh exec <id> <cmd>  # Exec a command via REST API

set -euo pipefail

AGENT_URL="${AGENT_URL:-http://localhost:8080}"
API_KEY="${API_KEY:-xgen_dev_key}"
SANDBOX_NS="${SANDBOX_NS:-xgen-sandboxes}"
AGENT_NS="${AGENT_NS:-xgen-system}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

header() { echo -e "\n${CYAN}=== $1 ===${NC}"; }
ok()     { echo -e "${GREEN}[OK]${NC} $1"; }
warn()   { echo -e "${YELLOW}[WARN]${NC} $1"; }
fail()   { echo -e "${RED}[FAIL]${NC} $1"; }

get_token() {
  curl -sf -X POST "$AGENT_URL/api/v1/auth/token" \
    -H "Content-Type: application/json" \
    -d "{\"api_key\":\"$API_KEY\"}" | jq -r '.token'
}

# --- No args: overview ---
if [ $# -eq 0 ]; then
  header "Agent Status"
  if curl -sf "$AGENT_URL/healthz" > /dev/null 2>&1; then
    ok "Agent is healthy at $AGENT_URL"
  else
    fail "Agent is not reachable at $AGENT_URL"
    exit 1
  fi

  header "Agent Pod"
  kubectl get pods -n "$AGENT_NS" -l app.kubernetes.io/name=xgen-agent -o wide 2>/dev/null || \
    kubectl get pods -n "$AGENT_NS" -o wide

  header "Sandbox Pods"
  kubectl get pods -n "$SANDBOX_NS" -o wide 2>/dev/null || echo "(no pods)"

  header "Sandboxes via API"
  TOKEN=$(get_token)
  curl -sf "$AGENT_URL/api/v1/sandboxes" \
    -H "Authorization: Bearer $TOKEN" | jq '.[].id' 2>/dev/null || echo "(none)"

  header "Recent Agent Logs (last 20 lines)"
  kubectl logs -n "$AGENT_NS" deployment/xgen-agent --tail=20 2>/dev/null || echo "(unavailable)"
  exit 0
fi

# --- exec subcommand ---
if [ "$1" = "exec" ]; then
  SANDBOX_ID="${2:?Usage: $0 exec <sandbox-id> <command>}"
  shift 2
  CMD="$1"
  shift
  ARGS=$(printf '%s\n' "$@" | jq -R . | jq -s .)

  TOKEN=$(get_token)
  echo -e "${CYAN}Exec in sandbox $SANDBOX_ID: $CMD $*${NC}"
  curl -sf -X POST "$AGENT_URL/api/v1/sandboxes/$SANDBOX_ID/exec" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d "{\"command\":\"$CMD\",\"args\":$ARGS,\"timeout\":10}" | jq .
  exit 0
fi

# --- Specific sandbox ---
SANDBOX_ID="$1"
POD_NAME="sbx-$SANDBOX_ID"

header "Sandbox $SANDBOX_ID"

# API status
TOKEN=$(get_token)
API_RESP=$(curl -sf "$AGENT_URL/api/v1/sandboxes/$SANDBOX_ID" \
  -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo '{}')

STATUS=$(echo "$API_RESP" | jq -r '.status // "unknown"')
PREVIEW=$(echo "$API_RESP" | jq -r '.preview_urls // {}')
echo "  Status: $STATUS"
echo "  Preview URLs: $PREVIEW"

# Pod status
header "Pod Status"
kubectl get pod "$POD_NAME" -n "$SANDBOX_NS" -o wide 2>/dev/null || {
  fail "Pod $POD_NAME not found"
  exit 1
}

header "Container Status"
kubectl get pod "$POD_NAME" -n "$SANDBOX_NS" -o jsonpath='{range .status.containerStatuses[*]}  {.name}: ready={.ready} restartCount={.restartCount} state={.state}
{end}' 2>/dev/null
echo

header "Sidecar Security Context (actual)"
kubectl get pod "$POD_NAME" -n "$SANDBOX_NS" \
  -o jsonpath='{.spec.containers[?(@.name=="sidecar")].securityContext}' 2>/dev/null | jq . 2>/dev/null || echo "(parse error)"

header "Sidecar Capabilities (runtime)"
kubectl exec -n "$SANDBOX_NS" "$POD_NAME" -c sidecar -- cat /proc/1/status 2>/dev/null | grep -i cap || echo "(unavailable)"

header "Sidecar Logs (last 30 lines)"
kubectl logs "$POD_NAME" -n "$SANDBOX_NS" -c sidecar --tail=30 2>/dev/null || echo "(unavailable)"

header "Runtime Container Logs"
kubectl logs "$POD_NAME" -n "$SANDBOX_NS" -c runtime --tail=10 2>/dev/null || echo "(unavailable)"

header "Process List (shared PID namespace)"
kubectl exec -n "$SANDBOX_NS" "$POD_NAME" -c sidecar -- ps aux 2>/dev/null || \
  kubectl exec -n "$SANDBOX_NS" "$POD_NAME" -c sidecar -- ls /proc 2>/dev/null | head -20 || echo "(unavailable)"

header "Test: exec echo hello"
EXEC_RESP=$(curl -sf -X POST "$AGENT_URL/api/v1/sandboxes/$SANDBOX_ID/exec" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"command":"echo","args":["hello"],"timeout":5}' 2>/dev/null || echo '{"error":"request failed"}')
echo "$EXEC_RESP" | jq .

EXIT_CODE=$(echo "$EXEC_RESP" | jq -r '.exit_code // "error"')
if [ "$EXIT_CODE" = "0" ]; then
  ok "Exec works!"
else
  fail "Exec failed (exit_code=$EXIT_CODE)"
  STDERR=$(echo "$EXEC_RESP" | jq -r '.stderr // ""')
  [ -n "$STDERR" ] && echo "  stderr: $STDERR"
fi

header "Network: listening ports in pod"
kubectl exec -n "$SANDBOX_NS" "$POD_NAME" -c sidecar -- cat /proc/net/tcp 2>/dev/null | head -5 || echo "(unavailable)"
