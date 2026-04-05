import { MsgType, encodePayload, decodePayload } from "./protocol/codec.js";
import { HttpTransport } from "./transport/http.js";
import { WsTransport } from "./transport/ws.js";
import type {
  SandboxInfo,
  SandboxStatus,
  ExecOptions,
  ExecResult,
  ExecEvent,
  TerminalOptions,
  FileInfo,
  FileEvent,
  Disposable,
} from "./types.js";

export class Sandbox {
  readonly id: string;
  readonly info: SandboxInfo;
  private http: HttpTransport;
  private ws: WsTransport | null = null;
  private _status: SandboxStatus;

  constructor(info: SandboxInfo, http: HttpTransport) {
    this.id = info.id;
    this.info = info;
    this.http = http;
    this._status = info.status;
  }

  get status(): SandboxStatus {
    return this._status;
  }

  get previewUrls(): Record<number, string> {
    return this.info.previewUrls;
  }

  /** Get the preview URL for a specific port */
  getPreviewUrl(port: number): string | undefined {
    return this.info.previewUrls[port];
  }

  /** Ensure WebSocket connection is established */
  private async ensureWs(): Promise<WsTransport> {
    if (this.ws) return this.ws;

    const wsUrl = this.http.getWsUrl(this.id);
    const token = this.http.getToken()!;
    this.ws = new WsTransport(wsUrl, token);
    await this.ws.connect();
    return this.ws;
  }

  /**
   * Execute a command and wait for it to complete.
   * Returns stdout, stderr, and exit code.
   */
  async exec(command: string, options?: ExecOptions): Promise<ExecResult> {
    const ws = await this.ensureWs();
    const channel = Date.now() & 0xffffffff;

    let stdout = "";
    let stderr = "";

    return new Promise<ExecResult>((resolve, reject) => {
      const timeout = options?.timeout ?? 30_000;
      const timer = setTimeout(() => {
        cleanupStdout();
        cleanupStderr();
        cleanupExit();
        reject(new Error("Exec timeout"));
      }, timeout);

      const cleanupStdout = ws.on(MsgType.ExecStdout, (env) => {
        if (env.channel === channel) {
          stdout += new TextDecoder().decode(env.payload);
        }
      });

      const cleanupStderr = ws.on(MsgType.ExecStderr, (env) => {
        if (env.channel === channel) {
          stderr += new TextDecoder().decode(env.payload);
        }
      });

      const cleanupExit = ws.on(MsgType.ExecExit, (env) => {
        if (env.channel === channel) {
          clearTimeout(timer);
          cleanupStdout();
          cleanupStderr();
          cleanupExit();
          cleanupError();
          const result = decodePayload<{ exit_code: number }>(env.payload);
          resolve({
            exitCode: result.exit_code,
            stdout,
            stderr,
          });
        }
      });

      const cleanupError = ws.on(MsgType.Error, (env) => {
        if (env.channel === channel || env.channel === 0) {
          clearTimeout(timer);
          cleanupStdout();
          cleanupStderr();
          cleanupExit();
          cleanupError();
          const error = decodePayload<{ code: string; message: string }>(env.payload);
          reject(new Error(`${error.code}: ${error.message}`));
        }
      });

      const payload = encodePayload({
        command: "sh",
        args: ["-c", command, ...(options?.args ?? [])],
        env: options?.env,
        cwd: options?.cwd,
        tty: false,
      });

      ws.send({
        type: MsgType.ExecStart,
        channel,
        id: 0,
        payload,
      });
    });
  }

  /**
   * Execute a command and stream output events.
   */
  async *execStream(
    command: string,
    options?: ExecOptions
  ): AsyncIterable<ExecEvent> {
    const ws = await this.ensureWs();
    const channel = Date.now() & 0xffffffff;

    const events: ExecEvent[] = [];
    let done = false;
    let resolver: (() => void) | null = null;

    const push = (event: ExecEvent) => {
      events.push(event);
      if (resolver) {
        resolver();
        resolver = null;
      }
    };

    const cleanupStdout = ws.on(MsgType.ExecStdout, (env) => {
      if (env.channel === channel) {
        push({ type: "stdout", data: new TextDecoder().decode(env.payload) });
      }
    });

    const cleanupStderr = ws.on(MsgType.ExecStderr, (env) => {
      if (env.channel === channel) {
        push({ type: "stderr", data: new TextDecoder().decode(env.payload) });
      }
    });

    const cleanupExit = ws.on(MsgType.ExecExit, (env) => {
      if (env.channel === channel) {
        const result = decodePayload<{ exit_code: number }>(env.payload);
        push({ type: "exit", exitCode: result.exit_code });
        done = true;
      }
    });

    const cleanupError = ws.on(MsgType.Error, (env) => {
      if (env.channel === channel || env.channel === 0) {
        const error = decodePayload<{ code: string; message: string }>(env.payload);
        push({ type: "stderr", data: `Error: ${error.code}: ${error.message}` });
        push({ type: "exit", exitCode: 1 });
        done = true;
      }
    });

    const payload = encodePayload({
      command: "sh",
      args: ["-c", command, ...(options?.args ?? [])],
      env: options?.env,
      cwd: options?.cwd,
      tty: false,
    });

    ws.send({ type: MsgType.ExecStart, channel, id: 0, payload });

    try {
      while (!done) {
        if (events.length === 0) {
          await new Promise<void>((resolve) => {
            resolver = resolve;
          });
        }
        while (events.length > 0) {
          yield events.shift()!;
        }
      }
    } finally {
      cleanupStdout();
      cleanupStderr();
      cleanupExit();
      cleanupError();
    }
  }

  /**
   * Open an interactive terminal session.
   */
  async openTerminal(options?: TerminalOptions): Promise<Terminal> {
    const ws = await this.ensureWs();
    const channel = Date.now() & 0xffffffff;

    const payload = encodePayload({
      command: "/bin/bash",
      args: [],
      tty: true,
      cols: options?.cols ?? 80,
      rows: options?.rows ?? 24,
      env: options?.env,
      cwd: options?.cwd,
    });

    ws.send({ type: MsgType.ExecStart, channel, id: 0, payload });

    return new Terminal(ws, channel);
  }

  // --- Filesystem ---

  async readFile(path: string): Promise<Uint8Array> {
    const ws = await this.ensureWs();
    const payload = encodePayload({ path });
    const resp = await ws.request(MsgType.FsRead, 0, payload);
    return resp.payload;
  }

  async readTextFile(path: string): Promise<string> {
    const data = await this.readFile(path);
    return new TextDecoder().decode(data);
  }

  async writeFile(path: string, content: Uint8Array | string): Promise<void> {
    const ws = await this.ensureWs();
    const bytes =
      typeof content === "string" ? new TextEncoder().encode(content) : content;
    const payload = encodePayload({ path, content: bytes });
    await ws.request(MsgType.FsWrite, 0, payload);
  }

  async listDir(path: string): Promise<FileInfo[]> {
    const ws = await this.ensureWs();
    const payload = encodePayload({ path });
    const resp = await ws.request(MsgType.FsList, 0, payload);
    return decodePayload<FileInfo[]>(resp.payload);
  }

  async removeFile(path: string, recursive = false): Promise<void> {
    const ws = await this.ensureWs();
    const payload = encodePayload({ path, recursive });
    await ws.request(MsgType.FsRemove, 0, payload);
  }

  /**
   * Watch a path for file changes.
   * Returns a Disposable to stop watching.
   */
  watchFiles(path: string, callback: (event: FileEvent) => void): Disposable {
    let disposed = false;
    let eventCleanup: (() => void) | null = null;
    let wsRef: WsTransport | null = null;

    this.ensureWs().then((ws) => {
      if (disposed) return;
      wsRef = ws;

      // Listen for file events
      eventCleanup = ws.on(MsgType.FsEvent, (env) => {
        const event = decodePayload<FileEvent>(env.payload);
        callback(event);
      });

      // Send watch request
      const payload = encodePayload({ path });
      ws.send({
        type: MsgType.FsWatch,
        channel: 0,
        id: 0,
        payload,
      });
    });

    return {
      dispose() {
        disposed = true;
        if (eventCleanup) {
          eventCleanup();
          eventCleanup = null;
        }
        // Send unwatch
        if (wsRef) {
          try {
            const payload = encodePayload({ path, unwatch: true });
            wsRef.send({
              type: MsgType.FsWatch,
              channel: 0,
              id: 0,
              payload,
            });
          } catch {
            // Ignore
          }
        }
      },
    };
  }

  // --- Port events ---

  onPortOpen(callback: (port: number) => void): Disposable {
    let disposed = false;
    this.ensureWs().then((ws) => {
      if (disposed) return;
      const cleanup = ws.on(MsgType.PortOpen, (env) => {
        const data = decodePayload<{ port: number }>(env.payload);
        callback(data.port);
      });
      // Store cleanup for dispose
      Object.assign(disposable, { _cleanup: cleanup });
    });

    const disposable: Disposable = {
      dispose() {
        disposed = true;
        const cleanup = (disposable as any)._cleanup;
        if (cleanup) cleanup();
      },
    };
    return disposable;
  }

  // --- Lifecycle ---

  async keepAlive(): Promise<void> {
    await this.http.keepAlive(this.id);
  }

  async destroy(): Promise<void> {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    await this.http.deleteSandbox(this.id);
    this._status = "stopped";
  }
}

export class Terminal {
  private ws: WsTransport;
  private channel: number;
  private dataHandlers: ((data: string) => void)[] = [];
  private cleanup: (() => void) | null = null;

  constructor(ws: WsTransport, channel: number) {
    this.ws = ws;
    this.channel = channel;

    this.cleanup = ws.on(MsgType.ExecStdout, (env) => {
      if (env.channel === this.channel) {
        const text = new TextDecoder().decode(env.payload);
        for (const handler of this.dataHandlers) {
          handler(text);
        }
      }
    });
  }

  write(data: string): void {
    const bytes = new TextEncoder().encode(data);
    // Prepend process ID (4 bytes, 0 for now since channel tracks it)
    const payload = new Uint8Array(4 + bytes.length);
    payload.set(bytes, 4);
    this.ws.send({
      type: MsgType.ExecStdin,
      channel: this.channel,
      id: 0,
      payload,
    });
  }

  onData(callback: (data: string) => void): Disposable {
    this.dataHandlers.push(callback);
    return {
      dispose: () => {
        const idx = this.dataHandlers.indexOf(callback);
        if (idx !== -1) this.dataHandlers.splice(idx, 1);
      },
    };
  }

  resize(cols: number, rows: number): void {
    const payload = encodePayload({
      cols,
      rows,
    });
    this.ws.send({
      type: MsgType.ExecResize,
      channel: this.channel,
      id: 0,
      payload,
    });
  }

  close(): void {
    if (this.cleanup) {
      this.cleanup();
      this.cleanup = null;
    }
    this.dataHandlers = [];
  }
}
