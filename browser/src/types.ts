export interface SandboxPreviewProps {
  /** Preview URL for the sandbox service */
  url: string;
  /** Optional title for the iframe */
  title?: string;
  /** CSS class name */
  className?: string;
  /** Inline styles */
  style?: React.CSSProperties;
  /** Whether to show the URL bar */
  showUrlBar?: boolean;
  /** Called when iframe loads */
  onLoad?: () => void;
  /** Called on load error */
  onError?: (error: string) => void;
}

export interface SandboxTerminalProps {
  /** WebSocket URL for the sandbox terminal */
  wsUrl: string;
  /** Authentication token */
  token: string;
  /** CSS class name */
  className?: string;
  /** Inline styles */
  style?: React.CSSProperties;
  /** Terminal columns (default: 80) */
  cols?: number;
  /** Terminal rows (default: 24) */
  rows?: number;
  /** Called when terminal connects */
  onConnect?: () => void;
  /** Called when terminal disconnects */
  onDisconnect?: () => void;
  /** Font size in pixels (default: 14) */
  fontSize?: number;
}

export interface FileEntry {
  name: string;
  size: number;
  isDir: boolean;
  modTime: number;
}

export interface SandboxFilesProps {
  /** Function to list directory contents */
  listDir: (path: string) => Promise<FileEntry[]>;
  /** Function to read a file's text content */
  readFile: (path: string) => Promise<string>;
  /** Optional: Function to write file content */
  writeFile?: (path: string, content: string) => Promise<void>;
  /** Optional: Function to delete a file */
  deleteFile?: (path: string) => Promise<void>;
  /** Initial path to display (default: ".") */
  initialPath?: string;
  /** CSS class name */
  className?: string;
  /** Inline styles */
  style?: React.CSSProperties;
  /** Called when a file is selected */
  onFileSelect?: (path: string, content: string) => void;
}
