import type { CreateSandboxOptions, SandboxInfo } from "../types.js";

export class XgenApiError extends Error {
  code?: string;
  retryable?: boolean;
  details?: Record<string, unknown>;
  requestId?: string;
  status: number;

  constructor(status: number, body: any, fallback: string) {
    const message = body?.message ?? body?.error ?? fallback;
    super(message);
    this.name = "XgenApiError";
    this.status = status;
    this.code = body?.code;
    this.retryable = body?.retryable;
    this.details = body?.details;
    this.requestId = body?.request_id;
  }
}

export class HttpTransport {
  private baseUrl: string;
  private apiKey: string;
  private apiVersion: "v1" | "v2";
  private token: string | null = null;
  private tokenExpiresAt: number = 0;

  constructor(agentUrl: string, apiKey: string, apiVersion: "v1" | "v2" = "v2") {
    this.baseUrl = agentUrl.replace(/\/$/, "");
    this.apiKey = apiKey;
    this.apiVersion = apiVersion;
  }

  private path(suffix: string): string {
    return `/api/${this.apiVersion}${suffix}`;
  }

  private async readError(resp: Response, fallback: string): Promise<XgenApiError> {
    const body = await resp.json().catch(async () => ({ message: await resp.text().catch(() => fallback) }));
    return new XgenApiError(resp.status, body, fallback);
  }

  private parseSandbox(data: any): SandboxInfo {
    const createdAtMs = data.created_at_ms;
    const expiresAtMs = data.expires_at_ms;
    return {
      id: data.id,
      status: data.status,
      template: data.template,
      wsUrl: data.ws_url,
      previewUrls: data.preview_urls ?? {},
      vncUrl: data.vnc_url,
      createdAt: data.created_at ?? (createdAtMs ? new Date(createdAtMs).toISOString() : ""),
      expiresAt: data.expires_at ?? (expiresAtMs ? new Date(expiresAtMs).toISOString() : ""),
      createdAtMs,
      expiresAtMs,
      metadata: data.metadata,
      capabilities: data.capabilities,
      fromWarmPool: data.from_warm_pool,
    };
  }

  private async ensureToken(): Promise<string> {
    // Use API key directly for simplicity; token exchange is optional
    if (this.token && Date.now() < this.tokenExpiresAt - 60_000) {
      return this.token;
    }

    const resp = await fetch(`${this.baseUrl}${this.path("/auth/token")}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ api_key: this.apiKey }),
    });

    if (!resp.ok) {
      throw await this.readError(resp, "Auth failed");
    }

    const data = await resp.json();
    this.token = data.token;
    this.tokenExpiresAt = data.expires_at_ms ?? new Date(data.expires_at).getTime();
    return this.token!;
  }

  private async headers(): Promise<Record<string, string>> {
    const token = await this.ensureToken();
    return {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    };
  }

  async createSandbox(options: CreateSandboxOptions): Promise<SandboxInfo> {
    const body =
      this.apiVersion === "v2"
        ? {
            template: options.template ?? "base",
            timeout_ms: options.timeoutMs ?? (options.timeoutSeconds ? options.timeoutSeconds * 1000 : undefined),
            resources: options.resources,
            env: options.env,
            ports: options.ports,
            gui: options.gui,
            metadata: options.metadata,
            capabilities: options.capabilities,
          }
        : {
            template: options.template ?? "base",
            timeout_seconds: options.timeoutSeconds ?? (options.timeoutMs ? Math.ceil(options.timeoutMs / 1000) : undefined),
            resources: options.resources,
            env: options.env,
            ports: options.ports,
            gui: options.gui,
            metadata: options.metadata,
            capabilities: options.capabilities,
          };

    const resp = await fetch(`${this.baseUrl}${this.path("/sandboxes")}`, {
      method: "POST",
      headers: await this.headers(),
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      throw await this.readError(resp, "Create sandbox failed");
    }

    return this.parseSandbox(await resp.json());
  }

  async getSandbox(id: string): Promise<SandboxInfo> {
    const resp = await fetch(`${this.baseUrl}${this.path(`/sandboxes/${id}`)}`, {
      headers: await this.headers(),
    });
    if (!resp.ok) {
      throw await this.readError(resp, `Get sandbox failed: sandbox '${id}' not found`);
    }
    return this.parseSandbox(await resp.json());
  }

  async listSandboxes(): Promise<SandboxInfo[]> {
    const resp = await fetch(`${this.baseUrl}${this.path("/sandboxes")}`, {
      headers: await this.headers(),
    });
    if (!resp.ok) {
      throw await this.readError(resp, "List sandboxes failed");
    }
    const data = await resp.json();
    return data.map((d: any) => this.parseSandbox(d));
  }

  async deleteSandbox(id: string): Promise<void> {
    const resp = await fetch(`${this.baseUrl}${this.path(`/sandboxes/${id}`)}`, {
      method: "DELETE",
      headers: await this.headers(),
    });
    if (!resp.ok && resp.status !== 204) {
      throw await this.readError(resp, `Delete sandbox '${id}' failed`);
    }
  }

  async keepAlive(id: string): Promise<void> {
    const resp = await fetch(
      `${this.baseUrl}${this.path(`/sandboxes/${id}/keepalive`)}`,
      {
        method: "POST",
        headers: await this.headers(),
      }
    );
    if (!resp.ok && resp.status !== 204) {
      throw await this.readError(resp, `Keepalive for sandbox '${id}' failed`);
    }
  }

  getWsUrl(id: string): string {
    const wsBase = this.baseUrl.replace(/^http/, "ws");
    return `${wsBase}${this.path(`/sandboxes/${id}/ws`)}`;
  }

  getToken(): string | null {
    return this.token;
  }
}
