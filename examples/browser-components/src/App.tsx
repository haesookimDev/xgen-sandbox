/**
 * Browser components example: Demonstrates all four React components from @xgen-sandbox/browser.
 *
 * Usage:
 *   npm install && npm run dev
 *
 * Prerequisites:
 *   - xgen-sandbox agent running (make dev-deploy)
 *   - Build SDKs first: cd ../../sdks/typescript && npm run build
 *   - Build browser: cd ../../browser && npm run build
 */

import { useCallback, useEffect, useRef, useState } from "react";
import {
  SandboxPreview,
  SandboxTerminal,
  SandboxDesktop,
  SandboxFiles,
} from "@xgen-sandbox/browser";
import { XgenClient, Sandbox } from "@xgen-sandbox/sdk";

const API_KEY = "xgen-local-api-key-2026";
const AGENT_URL = "http://localhost:8080";

type Tab = "preview" | "terminal" | "desktop" | "files";

async function fetchToken(): Promise<string> {
  const res = await fetch(`${AGENT_URL}/api/v1/auth/token`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ api_key: API_KEY }),
  });
  const data = await res.json();
  return data.token;
}

export function App() {
  const [tab, setTab] = useState<Tab>("terminal");
  const [sandbox, setSandbox] = useState<Sandbox | null>(null);
  const [token, setToken] = useState<string>("");
  const [status, setStatus] = useState("Initializing...");
  const clientRef = useRef<XgenClient | null>(null);

  useEffect(() => {
    let destroyed = false;

    async function init() {
      try {
        setStatus("Fetching auth token...");
        const jwt = await fetchToken();
        if (destroyed) return;
        setToken(jwt);

        const client = new XgenClient({ apiKey: API_KEY, agentUrl: AGENT_URL });
        clientRef.current = client;

        // Reuse existing sandbox from session if available
        const savedId = sessionStorage.getItem("xgen-sandbox-id");
        let sbx: Sandbox | null = null;

        if (savedId) {
          try {
            setStatus(`Reconnecting to sandbox ${savedId}...`);
            sbx = await client.getSandbox(savedId);
          } catch {
            sessionStorage.removeItem("xgen-sandbox-id");
            sbx = null;
          }
        }

        if (!sbx) {
          setStatus("Creating sandbox...");
          sbx = await client.createSandbox({
            template: "nodejs",
            gui: true,
            ports: [3000],
            timeoutSeconds: 600,
          });
          sessionStorage.setItem("xgen-sandbox-id", sbx.id);
        }

        // Ensure server.js exists and is running
        const check = await sbx.exec(
          "curl --connect-timeout 3 --max-time 5 -s -o /dev/null -w '%{http_code}' http://localhost:3000 2>/dev/null || echo 'down'",
          { timeout: 10_000 }
        );
        if (check.stdout.trim() !== "200") {
          await sbx.writeFile(
            "server.js",
            `const http = require('http');
http.createServer((req, res) => {
  res.writeHead(200, { 'Content-Type': 'text/html' });
  res.end('<h1>Hello from xgen-sandbox!</h1>');
}).listen(3000, '0.0.0.0', () => console.log('Server on port 3000'));`
          );

          const startServer = async () => {
            const iter = sbx.execStream("node server.js");
            for await (const event of iter) {
              if (event.type === "stdout" && event.data?.includes("Server on port 3000")) {
                return;
              }
            }
          };

          await Promise.race([
            startServer(),
            new Promise<never>((_, reject) =>
              setTimeout(() => reject(new Error("Server start timeout")), 30_000)
            ),
          ]);
        }

        if (destroyed) {
          return;
        }

        setSandbox(sbx);
        setStatus(`Sandbox ready: ${sbx.id}`);
      } catch (err) {
        if (!destroyed) {
          setStatus(`Error: ${err instanceof Error ? err.message : err}`);
        }
      }
    }

    init();

    return () => {
      destroyed = true;
    };
  }, []);

  const listDir = useCallback(
    async (path: string) => {
      if (!sandbox) return [];
      const items = await sandbox.listDir(path);
      return items.map((f) => ({
        name: f.name,
        size: f.size,
        isDir: f.isDir,
        modTime: f.modTime,
      }));
    },
    [sandbox]
  );

  const readFile = useCallback(
    async (path: string) => {
      if (!sandbox) return "";
      return sandbox.readTextFile(path);
    },
    [sandbox]
  );

  const writeFile = useCallback(
    async (path: string, content: string) => {
      if (!sandbox) return;
      await sandbox.writeFile(path, content);
    },
    [sandbox]
  );

  const deleteFile = useCallback(
    async (path: string) => {
      if (!sandbox) return;
      await sandbox.removeFile(path);
    },
    [sandbox]
  );

  const tabs: { key: Tab; label: string }[] = [
    { key: "terminal", label: "Terminal" },
    { key: "preview", label: "Preview" },
    { key: "desktop", label: "Desktop" },
    { key: "files", label: "Files" },
  ];

  return (
    <div style={{ display: "flex", flexDirection: "column", height: "100vh" }}>
      {/* Header */}
      <div
        style={{
          padding: "12px 20px",
          backgroundColor: "#1e293b",
          color: "#f1f5f9",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        }}
      >
        <span style={{ fontWeight: 600, fontSize: "16px" }}>
          xgen-sandbox Browser Components
        </span>
        <span style={{ fontSize: "13px", color: "#94a3b8" }}>{status}</span>
      </div>

      {/* Tabs */}
      <div
        style={{
          display: "flex",
          gap: "0",
          borderBottom: "1px solid #e2e8f0",
          backgroundColor: "#f8fafc",
        }}
      >
        {tabs.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            style={{
              padding: "10px 20px",
              border: "none",
              borderBottom: tab === t.key ? "2px solid #3b82f6" : "2px solid transparent",
              backgroundColor: "transparent",
              color: tab === t.key ? "#3b82f6" : "#64748b",
              fontWeight: tab === t.key ? 600 : 400,
              cursor: "pointer",
              fontSize: "14px",
            }}
          >
            {t.label}
          </button>
        ))}
      </div>

      {/* Content */}
      <div style={{ flex: 1, overflow: "hidden" }}>
        {!sandbox ? (
          <div
            style={{
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              height: "100%",
              color: "#64748b",
            }}
          >
            {status}
          </div>
        ) : (
          <>
            {tab === "terminal" && (
              <SandboxTerminal
                wsUrl={sandbox.info.wsUrl}
                token={token}
                onConnect={() => console.log("Terminal connected")}
                onDisconnect={() => console.log("Terminal disconnected")}
              />
            )}

            {tab === "preview" && sandbox.getPreviewUrl(3000) && (
              <SandboxPreview
                url={sandbox.getPreviewUrl(3000)!}
                showUrlBar={true}
                onLoad={() => console.log("Preview loaded")}
                onError={(err) => console.error("Preview error:", err)}
              />
            )}

            {tab === "desktop" && sandbox.info.vncUrl && (
              <SandboxDesktop
                vncUrl={sandbox.info.vncUrl}
                scaleViewport={true}
                onConnect={() => console.log("VNC connected")}
                onDisconnect={(d) => console.log("VNC disconnected:", d)}
              />
            )}

            {tab === "files" && (
              <SandboxFiles
                listDir={listDir}
                readFile={readFile}
                writeFile={writeFile}
                deleteFile={deleteFile}
                onFileSelect={(path, content) =>
                  console.log(`Selected: ${path} (${content.length} chars)`)
                }
              />
            )}
          </>
        )}
      </div>
    </div>
  );
}
