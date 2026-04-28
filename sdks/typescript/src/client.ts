import { HttpTransport } from "./transport/http.js";
import { Sandbox } from "./sandbox.js";
import type {
  XgenClientOptions,
  CreateSandboxOptions,
  SandboxInfo,
} from "./types.js";

export class XgenClient {
  private http: HttpTransport;

  constructor(options: XgenClientOptions) {
    this.http = new HttpTransport(options.agentUrl, options.apiKey, options.apiVersion ?? "v2");
  }

  /**
   * Create a new sandbox and return a Sandbox instance.
   * Waits for the sandbox to reach "running" status.
   */
  async createSandbox(options: CreateSandboxOptions = {}): Promise<Sandbox> {
    const info = await this.http.createSandbox(options);
    const sandbox = new Sandbox(info, this.http);

    // Poll until sandbox is running (or timeout)
    if (info.status !== "running") {
      await this.waitForRunning(info.id, 60_000);
      const updated = await this.http.getSandbox(info.id);
      return new Sandbox(updated, this.http);
    }

    return sandbox;
  }

  /** Get an existing sandbox by ID. */
  async getSandbox(id: string): Promise<Sandbox> {
    const info = await this.http.getSandbox(id);
    return new Sandbox(info, this.http);
  }

  /** List all sandboxes. */
  async listSandboxes(): Promise<SandboxInfo[]> {
    return this.http.listSandboxes();
  }

  private async waitForRunning(id: string, timeout: number): Promise<void> {
    const start = Date.now();
    while (Date.now() - start < timeout) {
      const info = await this.http.getSandbox(id);
      if (info.status === "running") return;
      if (info.status === "error" || info.status === "stopped") {
        throw new Error(`Sandbox ${id} entered ${info.status} state`);
      }
      await new Promise((r) => setTimeout(r, 1000));
    }
    throw new Error(`Sandbox ${id} did not become ready within ${timeout}ms`);
  }
}
