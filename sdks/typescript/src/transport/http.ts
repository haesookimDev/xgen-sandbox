import type { CreateSandboxOptions, SandboxInfo } from "../types.js";

export class HttpTransport {
  private baseUrl: string;
  private apiKey: string;
  private token: string | null = null;
  private tokenExpiresAt: number = 0;

  constructor(agentUrl: string, apiKey: string) {
    this.baseUrl = agentUrl.replace(/\/$/, "");
    this.apiKey = apiKey;
  }

  private async ensureToken(): Promise<string> {
    // Use API key directly for simplicity; token exchange is optional
    if (this.token && Date.now() < this.tokenExpiresAt - 60_000) {
      return this.token;
    }

    const resp = await fetch(`${this.baseUrl}/api/v1/auth/token`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ api_key: this.apiKey }),
    });

    if (!resp.ok) {
      throw new Error(`Auth failed (POST ${this.baseUrl}/api/v1/auth/token): ${resp.status} ${await resp.text()}`);
    }

    const data = await resp.json();
    this.token = data.token;
    this.tokenExpiresAt = new Date(data.expires_at).getTime();
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
    const resp = await fetch(`${this.baseUrl}/api/v1/sandboxes`, {
      method: "POST",
      headers: await this.headers(),
      body: JSON.stringify({
        template: options.template ?? "base",
        timeout_seconds: options.timeoutSeconds,
        resources: options.resources,
        env: options.env,
        ports: options.ports,
        gui: options.gui,
        metadata: options.metadata,
      }),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(`Create sandbox failed (${resp.status}): ${err.error}`);
    }

    const data = await resp.json();
    return {
      id: data.id,
      status: data.status,
      template: data.template,
      wsUrl: data.ws_url,
      previewUrls: data.preview_urls ?? {},
      vncUrl: data.vnc_url,
      createdAt: data.created_at,
      expiresAt: data.expires_at,
      metadata: data.metadata,
    };
  }

  async getSandbox(id: string): Promise<SandboxInfo> {
    const resp = await fetch(`${this.baseUrl}/api/v1/sandboxes/${id}`, {
      headers: await this.headers(),
    });
    if (!resp.ok) {
      throw new Error(`Get sandbox failed: sandbox '${id}' not found (${resp.status})`);
    }
    const data = await resp.json();
    return {
      id: data.id,
      status: data.status,
      template: data.template,
      wsUrl: data.ws_url,
      previewUrls: data.preview_urls ?? {},
      vncUrl: data.vnc_url,
      createdAt: data.created_at,
      expiresAt: data.expires_at,
      metadata: data.metadata,
    };
  }

  async listSandboxes(): Promise<SandboxInfo[]> {
    const resp = await fetch(`${this.baseUrl}/api/v1/sandboxes`, {
      headers: await this.headers(),
    });
    if (!resp.ok) {
      throw new Error(`List sandboxes failed: ${resp.status}`);
    }
    const data = await resp.json();
    return data.map((d: any) => ({
      id: d.id,
      status: d.status,
      template: d.template,
      wsUrl: d.ws_url,
      previewUrls: d.preview_urls ?? {},
      createdAt: d.created_at,
      expiresAt: d.expires_at,
    }));
  }

  async deleteSandbox(id: string): Promise<void> {
    const resp = await fetch(`${this.baseUrl}/api/v1/sandboxes/${id}`, {
      method: "DELETE",
      headers: await this.headers(),
    });
    if (!resp.ok && resp.status !== 204) {
      throw new Error(`Delete sandbox '${id}' failed (${resp.status})`);
    }
  }

  async keepAlive(id: string): Promise<void> {
    const resp = await fetch(
      `${this.baseUrl}/api/v1/sandboxes/${id}/keepalive`,
      {
        method: "POST",
        headers: await this.headers(),
      }
    );
    if (!resp.ok && resp.status !== 204) {
      throw new Error(`Keepalive for sandbox '${id}' failed (${resp.status})`);
    }
  }

  getWsUrl(id: string): string {
    const wsBase = this.baseUrl.replace(/^http/, "ws");
    return `${wsBase}/api/v1/sandboxes/${id}/ws`;
  }

  getToken(): string | null {
    return this.token;
  }
}
