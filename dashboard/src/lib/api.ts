const AGENT_URL =
  typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_AGENT_URL || "http://localhost:8080"
    : process.env.NEXT_PUBLIC_AGENT_URL || "http://localhost:8080";
const API_VERSION = process.env.NEXT_PUBLIC_AGENT_API_VERSION || "v2";
const API_BASE = `/api/${API_VERSION}`;

export interface Sandbox {
  id: string;
  status: string;
  template: string;
  ws_url: string;
  preview_urls?: Record<number, string>;
  vnc_url?: string;
  created_at: string;
  expires_at: string;
  created_at_ms?: number;
  expires_at_ms?: number;
  metadata?: Record<string, string>;
  capabilities?: string[];
  from_warm_pool?: boolean;
}

export interface CreateSandboxRequest {
  template: string;
  timeout_seconds?: number;
  timeout_ms?: number;
  ports?: number[];
  gui?: boolean;
  env?: Record<string, string>;
  metadata?: Record<string, string>;
  capabilities?: string[];
}

export interface AdminSummary {
  active_sandboxes: number;
  warm_pool: Record<string, { available: number; target: number }>;
  sandboxes_by_status: Record<string, number>;
  sandboxes_by_template: Record<string, number>;
}

export interface AdminMetrics {
  active_sandboxes: number;
}

export interface WarmPoolDetail {
  template: string;
  available: number;
  target: number;
}

export interface AuditEntry {
  timestamp: string;
  action: string;
  subject: string;
  role: string;
  status: number;
  remote_ip: string;
  sandbox_id?: string;
}

export interface AuditLogsResponse {
  entries: AuditEntry[];
  total: number;
}

async function apiFetch<T>(
  path: string,
  token: string,
  options?: RequestInit
): Promise<T> {
  const res = await fetch(`${AGENT_URL}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      ...options?.headers,
    },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(body.message || body.error || `API error: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export async function exchangeToken(
  apiKey: string
): Promise<{ token: string; expires_at?: string; expires_at_ms?: number }> {
  const res = await fetch(`${AGENT_URL}${API_BASE}/auth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ api_key: apiKey }),
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: "Invalid API key" }));
    throw new Error(body.error);
  }
  return res.json();
}

function sandboxPath(path = "") {
  return `${API_BASE}/sandboxes${path}`;
}

function normalizeCreateRequest(req: CreateSandboxRequest) {
  if (API_VERSION === "v2") {
    const { timeout_seconds, ...rest } = req;
    return {
      ...rest,
      timeout_ms: req.timeout_ms ?? (timeout_seconds ? timeout_seconds * 1000 : undefined),
    };
  }
  const { timeout_ms, ...rest } = req;
  return {
    ...rest,
    timeout_seconds: req.timeout_seconds ?? (timeout_ms ? Math.ceil(timeout_ms / 1000) : undefined),
  };
}

function normalizeSandbox(sbx: Sandbox): Sandbox {
  return {
    ...sbx,
    created_at: sbx.created_at ?? (sbx.created_at_ms ? new Date(sbx.created_at_ms).toISOString() : ""),
    expires_at: sbx.expires_at ?? (sbx.expires_at_ms ? new Date(sbx.expires_at_ms).toISOString() : ""),
  };
}

export function createApi(token: string) {
  return {
    listSandboxes: () =>
      apiFetch<Sandbox[]>(sandboxPath(), token).then((items) => items.map(normalizeSandbox)),

    getSandbox: (id: string) =>
      apiFetch<Sandbox>(sandboxPath(`/${id}`), token).then(normalizeSandbox),

    createSandbox: (req: CreateSandboxRequest) =>
      apiFetch<Sandbox>(sandboxPath(), token, {
        method: "POST",
        body: JSON.stringify(normalizeCreateRequest(req)),
      }).then(normalizeSandbox),

    deleteSandbox: (id: string) =>
      apiFetch<void>(sandboxPath(`/${id}`), token, {
        method: "DELETE",
      }),

    keepalive: (id: string) =>
      apiFetch<void>(sandboxPath(`/${id}/keepalive`), token, {
        method: "POST",
      }),

    // Admin endpoints
    getAdminSummary: () =>
      apiFetch<AdminSummary>(`${API_BASE}/admin/summary`, token),

    getAdminMetrics: () =>
      apiFetch<AdminMetrics>(`${API_BASE}/admin/metrics`, token),

    getAdminWarmPool: () =>
      apiFetch<{ pools: WarmPoolDetail[] }>(`${API_BASE}/admin/warm-pool`, token),

    getAuditLogs: (params?: {
      limit?: number;
      offset?: number;
      action?: string;
      subject?: string;
    }) => {
      const qs = new URLSearchParams();
      if (params?.limit) qs.set("limit", String(params.limit));
      if (params?.offset) qs.set("offset", String(params.offset));
      if (params?.action) qs.set("action", params.action);
      if (params?.subject) qs.set("subject", params.subject);
      return apiFetch<AuditLogsResponse>(
        `${API_BASE}/admin/audit-logs?${qs.toString()}`,
        token
      );
    },
  };
}
