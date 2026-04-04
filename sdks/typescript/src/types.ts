export type SandboxStatus = "starting" | "running" | "stopping" | "stopped" | "error";

export interface CreateSandboxOptions {
  template?: string;
  timeoutSeconds?: number;
  resources?: {
    cpu?: string;
    memory?: string;
    disk?: string;
  };
  env?: Record<string, string>;
  ports?: number[];
  gui?: boolean;
  metadata?: Record<string, string>;
}

export interface SandboxInfo {
  id: string;
  status: SandboxStatus;
  template: string;
  wsUrl: string;
  previewUrls: Record<number, string>;
  vncUrl?: string;
  createdAt: string;
  expiresAt: string;
  metadata?: Record<string, string>;
}

export interface ExecOptions {
  args?: string[];
  env?: Record<string, string>;
  cwd?: string;
  timeout?: number;
}

export interface ExecResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

export interface ExecEvent {
  type: "stdout" | "stderr" | "exit";
  data?: string;
  exitCode?: number;
}

export interface TerminalOptions {
  cols?: number;
  rows?: number;
  env?: Record<string, string>;
  cwd?: string;
}

export interface FileInfo {
  name: string;
  size: number;
  isDir: boolean;
  modTime: number;
}

export interface Disposable {
  dispose(): void;
}

export interface XgenClientOptions {
  apiKey: string;
  agentUrl: string;
}
