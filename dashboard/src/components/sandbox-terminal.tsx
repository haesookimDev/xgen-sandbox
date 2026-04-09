"use client";

import { useEffect, useRef } from "react";

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

function encodeMsgpack(obj: Record<string, unknown>): Uint8Array {
  const entries = Object.entries(obj);
  const parts: number[] = [];
  parts.push(0x80 | entries.length);

  for (const [key, value] of entries) {
    const keyBytes = new TextEncoder().encode(key);
    if (keyBytes.length < 32) {
      parts.push(0xa0 | keyBytes.length);
    } else {
      parts.push(0xd9, keyBytes.length);
    }
    parts.push(...keyBytes);

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
        const fb = new ArrayBuffer(9);
        const fv = new DataView(fb);
        fv.setUint8(0, 0xcb);
        fv.setFloat64(1, value as number, false);
        parts.push(...new Uint8Array(fb));
      }
    } else if (Array.isArray(value)) {
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

interface SandboxTerminalProps {
  wsUrl: string;
  token: string;
}

export function SandboxTerminal({ wsUrl, token }: SandboxTerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const cleanupRef = useRef<(() => void) | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    let destroyed = false;

    async function init() {
      const [{ Terminal }, { FitAddon }] = await Promise.all([
        import("@xterm/xterm"),
        import("@xterm/addon-fit"),
      ]);

      if (destroyed || !containerRef.current) return;

      const term = new Terminal({
        cols: 80,
        rows: 24,
        fontSize: 14,
        fontFamily: "'Menlo', 'Monaco', 'Courier New', monospace",
        cursorBlink: true,
        theme: {
          background: "#1e1e2e",
          foreground: "#cdd6f4",
          cursor: "#f5e0dc",
        },
      });

      const fitAddon = new FitAddon();
      term.loadAddon(fitAddon);
      term.open(containerRef.current);
      fitAddon.fit();

      term.writeln("Connecting to sandbox...");

      const fullUrl = `${wsUrl}?token=${encodeURIComponent(token)}`;
      const ws = new WebSocket(fullUrl);
      ws.binaryType = "arraybuffer";

      const channel = (Date.now() & 0xffffffff) >>> 0;

      ws.onopen = () => {
        term.writeln("Connected. Starting shell...\r\n");
      };

      ws.onmessage = (event) => {
        const data = new Uint8Array(event.data as ArrayBuffer);
        if (data.length < HEADER_SIZE) return;
        const msgType = data[0];

        if (msgType === MSG_SANDBOX_READY) {
          const payload = encodeMsgpack({
            command: "/bin/bash",
            args: [],
            tty: true,
            cols: term.cols,
            rows: term.rows,
          });
          ws.send(encodeEnvelope(MSG_EXEC_START, channel, 0, payload));
          return;
        }

        if (msgType === MSG_EXEC_STDOUT) {
          term.write(data.slice(HEADER_SIZE));
        }
      };

      ws.onclose = () => {
        if (!destroyed) term.writeln("\r\n\r\nDisconnected.");
      };

      ws.onerror = () => {
        term.writeln("\r\n\r\nConnection error.");
      };

      term.onData((data: string) => {
        if (ws.readyState !== WebSocket.OPEN) return;
        const textBytes = new TextEncoder().encode(data);
        const payload = new Uint8Array(4 + textBytes.length);
        payload.set(textBytes, 4);
        ws.send(encodeEnvelope(MSG_EXEC_STDIN, channel, 0, payload));
      });

      term.onResize(({ cols: c, rows: r }) => {
        if (ws.readyState !== WebSocket.OPEN) return;
        ws.send(
          encodeEnvelope(MSG_EXEC_RESIZE, channel, 0, encodeMsgpack({ cols: c, rows: r }))
        );
      });

      const observer = new ResizeObserver(() => fitAddon.fit());
      observer.observe(containerRef.current!);

      cleanupRef.current = () => {
        observer.disconnect();
        ws.close();
        term.dispose();
      };
    }

    init();

    return () => {
      destroyed = true;
      cleanupRef.current?.();
    };
  }, [wsUrl, token]);

  return (
    <div
      ref={containerRef}
      style={{
        width: "100%",
        height: "500px",
        backgroundColor: "#1e1e2e",
        borderRadius: "8px",
        overflow: "hidden",
        padding: "4px",
      }}
    />
  );
}
