import { useCallback, useRef, useState } from "react";
import type { SandboxPreviewProps } from "../types.js";

export function SandboxPreview({
  url,
  title = "Sandbox Preview",
  className,
  style,
  showUrlBar = true,
  onLoad,
  onError,
}: SandboxPreviewProps) {
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const [currentUrl, setCurrentUrl] = useState(url);
  const [inputUrl, setInputUrl] = useState(url);
  const [loading, setLoading] = useState(true);

  const handleLoad = useCallback(() => {
    setLoading(false);
    onLoad?.();
  }, [onLoad]);

  const handleError = useCallback(() => {
    setLoading(false);
    onError?.("Failed to load preview");
  }, [onError]);

  const handleNavigate = useCallback(() => {
    setCurrentUrl(inputUrl);
    setLoading(true);
  }, [inputUrl]);

  const handleRefresh = useCallback(() => {
    if (iframeRef.current) {
      setLoading(true);
      iframeRef.current.src = currentUrl;
    }
  }, [currentUrl]);

  return (
    <div
      className={className}
      style={{
        display: "flex",
        flexDirection: "column",
        width: "100%",
        height: "100%",
        border: "1px solid #e2e8f0",
        borderRadius: "8px",
        overflow: "hidden",
        backgroundColor: "#fff",
        ...style,
      }}
    >
      {showUrlBar && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "8px",
            padding: "8px 12px",
            backgroundColor: "#f8fafc",
            borderBottom: "1px solid #e2e8f0",
            fontSize: "13px",
          }}
        >
          <button
            onClick={handleRefresh}
            style={{
              border: "none",
              background: "none",
              cursor: "pointer",
              padding: "4px 8px",
              borderRadius: "4px",
              fontSize: "14px",
            }}
            title="Refresh"
          >
            &#x21bb;
          </button>
          <input
            type="text"
            value={inputUrl}
            onChange={(e) => setInputUrl(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleNavigate()}
            style={{
              flex: 1,
              padding: "4px 8px",
              border: "1px solid #e2e8f0",
              borderRadius: "4px",
              fontSize: "13px",
              fontFamily: "monospace",
              backgroundColor: "#fff",
              outline: "none",
            }}
          />
        </div>
      )}

      <div style={{ flex: 1, position: "relative" }}>
        {loading && (
          <div
            style={{
              position: "absolute",
              inset: 0,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              backgroundColor: "#f8fafc",
              zIndex: 1,
              fontSize: "14px",
              color: "#64748b",
            }}
          >
            Loading...
          </div>
        )}
        <iframe
          ref={iframeRef}
          src={currentUrl}
          title={title}
          onLoad={handleLoad}
          onError={handleError}
          style={{
            width: "100%",
            height: "100%",
            border: "none",
          }}
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals"
        />
      </div>
    </div>
  );
}
