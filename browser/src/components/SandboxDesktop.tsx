import { useEffect, useRef } from "react";
import type { SandboxDesktopProps } from "../types.js";

export function SandboxDesktop({
  vncUrl,
  className,
  style,
  viewOnly = false,
  scaleViewport = true,
  onConnect,
  onDisconnect,
}: SandboxDesktopProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const rfbRef = useRef<any>(null);
  const initRef = useRef(false);

  useEffect(() => {
    if (!containerRef.current || initRef.current) return;
    initRef.current = true;

    let destroyed = false;

    async function init() {
      const { default: RFB } = await import("@novnc/novnc/core/rfb.js");

      if (destroyed || !containerRef.current) return;

      // Convert https:// to wss:// (or http:// to ws://)
      const wsUrl = vncUrl
        .replace(/^https:/, "wss:")
        .replace(/^http:/, "ws:");

      const rfb = new RFB(containerRef.current, wsUrl);
      rfb.scaleViewport = scaleViewport;
      rfb.viewOnly = viewOnly;
      rfb.resizeSession = false;
      rfbRef.current = rfb;

      rfb.addEventListener("connect", () => {
        if (!destroyed) onConnect?.();
      });

      rfb.addEventListener("disconnect", (e: CustomEvent) => {
        if (!destroyed) onDisconnect?.({ clean: e.detail?.clean ?? false });
      });

      rfb.addEventListener("credentialsrequired", () => {
        // VNC server runs with -nopw, send empty password
        rfb.sendCredentials({ password: "" });
      });
    }

    init();

    return () => {
      destroyed = true;
      rfbRef.current?.disconnect();
      rfbRef.current = null;
      initRef.current = false;
    };
  }, [vncUrl, viewOnly, scaleViewport, onConnect, onDisconnect]);

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
        ...style,
      }}
    />
  );
}
