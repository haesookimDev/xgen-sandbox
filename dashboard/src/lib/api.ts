const AGENT_URL =
  typeof window !== "undefined"
    ? process.env.NEXT_PUBLIC_AGENT_URL || "http://localhost:8080"
    : process.env.NEXT_PUBLIC_AGENT_URL || "http://localhost:8080";

export interface Sandbox {
  id: string;
  status: string;
  template: string;
  ws_url: string;
  preview_urls?: Record<number, string>;
  vnc_url?: string;
  created_at: string;
  expires_at: string;
  metadata?: Record<string, string>;
}

export interface CreateSandboxRequest {
  template: string;
  timeout_seconds?: number;
  ports?: number[];
  gui?: boolean;
  env?: Record<string, string>;
  metadata?: Record<string, string>;
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
    throw new Error(body.error || `API error: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json();
}

export async function exchangeToken(
  apiKey: string
): Promise<{ token: string; expires_at: string }> {
  const res = await fetch(`${AGENT_URL}/api/v1/auth/token`, {
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

export function createApi(token: string) {
  return {
    listSandboxes: () =>
      apiFetch<Sandbox[]>("/api/v1/sandboxes", token),

    getSandbox: (id: string) =>
      apiFetch<Sandbox>(`/api/v1/sandboxes/${id}`, token),

    createSandbox: (req: CreateSandboxRequest) =>
      apiFetch<Sandbox>("/api/v1/sandboxes", token, {
        method: "POST",
        body: JSON.stringify(req),
      }),

    deleteSandbox: (id: string) =>
      apiFetch<void>(`/api/v1/sandboxes/${id}`, token, {
        method: "DELETE",
      }),

    keepalive: (id: string) =>
      apiFetch<void>(`/api/v1/sandboxes/${id}/keepalive`, token, {
        method: "POST",
      }),

    // Admin endpoints
    getAdminSummary: () =>
      apiFetch<AdminSummary>("/api/v1/admin/summary", token),

    getAdminMetrics: () =>
      apiFetch<AdminMetrics>("/api/v1/admin/metrics", token),

    getAdminWarmPool: () =>
      apiFetch<{ pools: WarmPoolDetail[] }>("/api/v1/admin/warm-pool", token),

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
        `/api/v1/admin/audit-logs?${qs.toString()}`,
        token
      );
    },
  };
}
