import { useEffect, useRef } from "react";
import type { SandboxTerminalProps } from "../types.js";

// WebSocket binary protocol constants
const HEADER_SIZE = 9;
const MSG_EXEC_START = 0x20;
const MSG_EXEC_STDIN = 0x21;
const MSG_EXEC_STDOUT = 0x22;
const MSG_EXEC_RESIZE = 0x26;
const MSG_SANDBOX_READY = 0x50;

function encodeEnvelope(
  type: number,
  channel: number,
  id: number,
  payload: Uint8Array
): Uint8Array {
  const buf = new Uint8Array(HEADER_SIZE + payload.length);
  const view = new DataView(buf.buffer);
  buf[0] = type;
  view.setUint32(1, channel, false);
  view.setUint32(5, id, false);
  buf.set(payload, HEADER_SIZE);
  return buf;
}

/** Minimal msgpack encoder for simple objects */
function encodeMsgpack(obj: Record<string, unknown>): Uint8Array {
  // Use JSON as a simple fallback - the sidecar also supports msgpack
  // but for browser we encode manually for the simple payloads we need
  const entries = Object.entries(obj);
  const parts: number[] = [];

  // fixmap header
  parts.push(0x80 | entries.length);

  for (const [key, value] of entries) {
    // encode string key
    const keyBytes = new TextEncoder().encode(key);
    if (keyBytes.length < 32) {
      parts.push(0xa0 | keyBytes.length);
    } else {
      parts.push(0xd9, keyBytes.length);
    }
    parts.push(...keyBytes);

    // encode value
    if (typeof value === "string") {
      const valBytes = new TextEncoder().encode(value);
      if (valBytes.length < 32) {
        parts.push(0xa0 | valBytes.length);
      } else if (valBytes.length < 256) {
        parts.push(0xd9, valBytes.length);
      } else {
        parts.push(0xda, (valBytes.length >> 8) & 0xff, valBytes.length & 0xff);
      }
      parts.push(...valBytes);
    } else if (typeof value === "boolean") {
      parts.push(value ? 0xc3 : 0xc2);
    } else if (typeof value === "number") {
      if (Number.isInteger(value) && value >= 0 && value < 128) {
        parts.push(value);
      } else if (Number.isInteger(value) && value >= 0 && value < 65536) {
        parts.push(0xcd, (value >> 8) & 0xff, value & 0xff);
      } else {
        // float64
        const fb = new ArrayBuffer(9);
        const fv = new DataView(fb);
        fv.setUint8(0, 0xcb);
        fv.setFloat64(1, value, false);
        parts.push(...new Uint8Array(fb));
      }
    } else if (Array.isArray(value)) {
      // fixarray of strings
      parts.push(0x90 | value.length);
      for (const item of value) {
        const itemBytes = new TextEncoder().encode(String(item));
        if (itemBytes.length < 32) {
          parts.push(0xa0 | itemBytes.length);
        } else {
          parts.push(0xd9, itemBytes.length);
        }
        parts.push(...itemBytes);
      }
    }
  }

  return new Uint8Array(parts);
}

export function SandboxTerminal({
  wsUrl,
  token,
  className,
  style,
  cols = 80,
  rows = 24,
  fontSize = 14,
  onConnect,
  onDisconnect,
}: SandboxTerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<any>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<any>(null);
  const channelRef = useRef<number>(0);
  const initRef = useRef(false);

  useEffect(() => {
    if (!containerRef.current || initRef.current) return;
    initRef.current = true;

    let destroyed = false;

    async function init() {
      const [{ Terminal }, { FitAddon }] = await Promise.all([
        import("@xterm/xterm"),
        import("@xterm/addon-fit"),
      ]);

      if (destroyed) return;

      const term = new Terminal({
        cols,
        rows,
        fontSize,
        fontFamily: "'Menlo', 'Monaco', 'Courier New', monospace",
        cursorBlink: true,
        theme: {
          background: "#1e1e2e",
          foreground: "#cdd6f4",
          cursor: "#f5e0dc",
          selectionBackground: "#585b70",
          black: "#45475a",
          red: "#f38ba8",
          green: "#a6e3a1",
          yellow: "#f9e2af",
          blue: "#89b4fa",
          magenta: "#f5c2e7",
          cyan: "#94e2d5",
          white: "#bac2de",
          brightBlack: "#585b70",
          brightRed: "#f38ba8",
          brightGreen: "#a6e3a1",
          brightYellow: "#f9e2af",
          brightBlue: "#89b4fa",
          brightMagenta: "#f5c2e7",
          brightCyan: "#94e2d5",
          brightWhite: "#a6adc8",
        },
      });

      const fitAddon = new FitAddon();
      term.loadAddon(fitAddon);
      term.open(containerRef.current!);
      fitAddon.fit();

      termRef.current = term;
      fitAddonRef.current = fitAddon;

      term.writeln("Connecting to sandbox...");

      // Connect WebSocket
      const fullUrl = `${wsUrl}?token=${encodeURIComponent(token)}`;
      const ws = new WebSocket(fullUrl);
      ws.binaryType = "arraybuffer";
      wsRef.current = ws;

      const channel = (Date.now() & 0xffffffff) >>> 0;
      channelRef.current = channel;

      ws.onopen = () => {
        term.writeln("Connected. Starting shell...\r\n");
        onConnect?.();
      };

      ws.onmessage = (event) => {
        const data = new Uint8Array(event.data as ArrayBuffer);
        if (data.length < HEADER_SIZE) return;

        const msgType = data[0];

        if (msgType === MSG_SANDBOX_READY) {
          // Send ExecStart for bash shell
          const payload = encodeMsgpack({
            command: "/bin/bash",
            args: [],
            tty: true,
            cols,
            rows,
          });
          const envelope = encodeEnvelope(MSG_EXEC_START, channel, 0, payload);
          ws.send(envelope);
          return;
        }

        if (msgType === MSG_EXEC_STDOUT) {
          const payload = data.slice(HEADER_SIZE);
          term.write(payload);
        }
      };

      ws.onclose = () => {
        if (!destroyed) {
          term.writeln("\r\n\r\nDisconnected.");
          onDisconnect?.();
        }
      };

      ws.onerror = () => {
        term.writeln("\r\n\r\nConnection error.");
      };

      // Terminal input -> WebSocket
      term.onData((data: string) => {
        if (ws.readyState !== WebSocket.OPEN) return;

        const textBytes = new TextEncoder().encode(data);
        // ExecStdin payload: process_id (4 bytes) + data
        const payload = new Uint8Array(4 + textBytes.length);
        // process_id = 0 (will be resolved by channel)
        payload.set(textBytes, 4);

        const envelope = encodeEnvelope(MSG_EXEC_STDIN, channel, 0, payload);
        ws.send(envelope);
      });

      // Handle resize
      term.onResize(({ cols: c, rows: r }: { cols: number; rows: number }) => {
        if (ws.readyState !== WebSocket.OPEN) return;
        const payload = encodeMsgpack({ cols: c, rows: r });
        const envelope = encodeEnvelope(MSG_EXEC_RESIZE, channel, 0, payload);
        ws.send(envelope);
      });
    }

    init();

    // Handle container resize
    const observer = new ResizeObserver(() => {
      fitAddonRef.current?.fit();
    });
    if (containerRef.current) {
      observer.observe(containerRef.current);
    }

    return () => {
      destroyed = true;
      observer.disconnect();
      wsRef.current?.close();
      termRef.current?.dispose();
      initRef.current = false;
    };
  }, [wsUrl, token, cols, rows, fontSize, onConnect, onDisconnect]);

  return (
    <div
      ref={containerRef}
      className={className}
      style={{
        width: "100%",
        height: "100%",
        backgroundColor: "#1e1e2e",
        borderRadius: "8px",
        overflow: "hidden",
        padding: "4px",
        ...style,
      }}
    />
  );
}
