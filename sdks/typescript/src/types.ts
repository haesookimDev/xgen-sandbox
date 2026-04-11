/** Lifecycle status of a sandbox. */
export type SandboxStatus = "starting" | "running" | "stopping" | "stopped" | "error";

/** Options for creating a new sandbox. */
export interface CreateSandboxOptions {
  /** Runtime template (e.g. "base", "nodejs", "python", "gui"). Defaults to "base". */
  template?: string;
  /** Sandbox timeout in seconds. The sandbox is automatically destroyed after this duration. */
  timeoutSeconds?: number;
  /** Resource limits for the sandbox container. */
  resources?: {
    cpu?: string;
    memory?: string;
    disk?: string;
  };
  /** Environment variables injected into the sandbox runtime. */
  env?: Record<string, string>;
  /** Ports to expose via preview URLs. */
  ports?: number[];
  /** Enable GUI (VNC) desktop environment. */
  gui?: boolean;
  /** Arbitrary key-value metadata attached to the sandbox. */
  metadata?: Record<string, string>;
}

/** Runtime information about a sandbox instance. */
export interface SandboxInfo {
  /** Unique sandbox identifier. */
  id: string;
  /** Current lifecycle status. */
  status: SandboxStatus;
  /** Runtime template used to create this sandbox. */
  template: string;
  /** WebSocket URL for real-time communication with the sandbox sidecar. */
  wsUrl: string;
  /** Map of exposed port numbers to their public preview URLs. */
  previewUrls: Record<number, string>;
  /** VNC URL for GUI sandboxes. Only present when `gui: true`. */
  vncUrl?: string;
  /** ISO 8601 timestamp of when the sandbox was created. */
  createdAt: string;
  /** ISO 8601 timestamp of when the sandbox will expire. */
  expiresAt: string;
  /** User-defined metadata. */
  metadata?: Record<string, string>;
}

/** Options for command execution. */
export interface ExecOptions {
  /** Additional arguments appended after the command. */
  args?: string[];
  /** Environment variables for the command. */
  env?: Record<string, string>;
  /** Working directory. Defaults to `/home/sandbox/workspace`. */
  cwd?: string;
  /** Timeout in milliseconds. Defaults to 30000. */
  timeout?: number;
}

/** Result of a synchronous command execution. */
export interface ExecResult {
  /** Process exit code. 0 indicates success. */
  exitCode: number;
  /** Captured standard output. */
  stdout: string;
  /** Captured standard error. */
  stderr: string;
}

/** A streaming event emitted during command execution. */
export interface ExecEvent {
  /** Event type: "stdout" for output, "stderr" for errors, "exit" when process terminates. */
  type: "stdout" | "stderr" | "exit";
  /** Output data. Present for "stdout" and "stderr" events. */
  data?: string;
  /** Process exit code. Present for "exit" events. */
  exitCode?: number;
}

/** Options for opening an interactive terminal session. */
export interface TerminalOptions {
  /** Terminal width in columns. Defaults to 80. */
  cols?: number;
  /** Terminal height in rows. Defaults to 24. */
  rows?: number;
  /** Environment variables for the terminal shell. */
  env?: Record<string, string>;
  /** Working directory for the terminal shell. */
  cwd?: string;
}

/** Metadata about a file or directory entry. */
export interface FileInfo {
  /** File or directory name (not the full path). */
  name: string;
  /** File size in bytes. */
  size: number;
  /** True if this entry is a directory. */
  isDir: boolean;
  /** Last modification time as a Unix timestamp (seconds). */
  modTime: number;
}

/** A file system change event. */
export interface FileEvent {
  /** Path of the changed file relative to the workspace root. */
  path: string;
  /** Type of change that occurred. */
  type: "created" | "modified" | "deleted";
}

/** A handle that can be disposed to unsubscribe from events or release resources. */
export interface Disposable {
  /** Stop listening and release associated resources. */
  dispose(): void;
}

/** Configuration options for creating an XgenClient. */
export interface XgenClientOptions {
  /** API key for authentication with the xgen-sandbox agent. */
  apiKey: string;
  /** Base URL of the xgen-sandbox agent (e.g. "http://localhost:8080"). */
  agentUrl: string;
}
