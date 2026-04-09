"use client";

import { useState } from "react";
import dynamic from "next/dynamic";
import { useParams, useRouter } from "next/navigation";
import { useSandbox, useDeleteSandbox } from "@/hooks/use-sandboxes";
import { useAuth } from "@/providers/auth-provider";
import { createApi } from "@/lib/api";
import { StatusBadge } from "@/components/status-badge";
import { formatRelativeTime, formatTimeRemaining, cn } from "@/lib/utils";

const SandboxTerminal = dynamic(
  () => import("@/components/sandbox-terminal").then((m) => m.SandboxTerminal),
  { ssr: false, loading: () => <p className="text-sm text-muted-foreground p-4">Loading terminal...</p> }
);

const tabs = ["Info", "Preview", "Terminal"] as const;

export default function SandboxDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;
  const { data: sandbox, isLoading } = useSandbox(id);
  const deleteMutation = useDeleteSandbox();
  const { token } = useAuth();
  const [activeTab, setActiveTab] = useState<(typeof tabs)[number]>("Info");

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">Loading...</p>;
  }

  if (!sandbox) {
    return <p className="text-sm text-muted-foreground">Sandbox not found</p>;
  }

  const handleDelete = async () => {
    if (confirm(`Delete sandbox ${sandbox.id}?`)) {
      await deleteMutation.mutateAsync(sandbox.id);
      router.push("/dashboard/sandboxes");
    }
  };

  const handleKeepalive = async () => {
    if (token) {
      const api = createApi(token);
      await api.keepalive(sandbox.id);
    }
  };

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold font-mono">{sandbox.id}</h1>
          <div className="mt-1 flex items-center gap-3">
            <StatusBadge status={sandbox.status} />
            <span className="text-sm text-muted-foreground">
              {sandbox.template}
            </span>
          </div>
        </div>
        <div className="flex gap-2">
          <button
            onClick={handleKeepalive}
            className="rounded-md border px-3 py-1.5 text-sm hover:bg-accent"
          >
            Keep Alive
          </button>
          <button
            onClick={handleDelete}
            className="rounded-md border border-destructive/30 px-3 py-1.5 text-sm text-destructive hover:bg-destructive/10"
          >
            Delete
          </button>
        </div>
      </div>

      <div className="mb-4 flex gap-1 border-b">
        {tabs.map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={cn(
              "px-4 py-2 text-sm font-medium border-b-2 -mb-px transition-colors",
              activeTab === tab
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            )}
          >
            {tab}
          </button>
        ))}
      </div>

      {activeTab === "Info" && (
        <div className="space-y-4">
          <div className="rounded-lg border p-4">
            <h3 className="mb-3 text-sm font-semibold">Details</h3>
            <dl className="grid grid-cols-2 gap-2 text-sm">
              <dt className="text-muted-foreground">ID</dt>
              <dd className="font-mono">{sandbox.id}</dd>
              <dt className="text-muted-foreground">Status</dt>
              <dd>{sandbox.status}</dd>
              <dt className="text-muted-foreground">Template</dt>
              <dd>{sandbox.template}</dd>
              <dt className="text-muted-foreground">Created</dt>
              <dd>{formatRelativeTime(sandbox.created_at)}</dd>
              <dt className="text-muted-foreground">Expires</dt>
              <dd>{formatTimeRemaining(sandbox.expires_at)}</dd>
              <dt className="text-muted-foreground">WS URL</dt>
              <dd className="truncate font-mono text-xs">{sandbox.ws_url}</dd>
            </dl>
          </div>

          {sandbox.preview_urls &&
            Object.keys(sandbox.preview_urls).length > 0 && (
              <div className="rounded-lg border p-4">
                <h3 className="mb-3 text-sm font-semibold">Preview URLs</h3>
                <ul className="space-y-1 text-sm">
                  {Object.entries(sandbox.preview_urls).map(([port, url]) => (
                    <li key={port} className="flex items-center gap-2">
                      <span className="text-muted-foreground">
                        Port {port}:
                      </span>
                      <a
                        href={url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="font-mono text-xs text-primary hover:underline"
                      >
                        {url}
                      </a>
                    </li>
                  ))}
                </ul>
              </div>
            )}

          {sandbox.metadata && Object.keys(sandbox.metadata).length > 0 && (
            <div className="rounded-lg border p-4">
              <h3 className="mb-3 text-sm font-semibold">Metadata</h3>
              <dl className="grid grid-cols-2 gap-2 text-sm">
                {Object.entries(sandbox.metadata).map(([k, v]) => (
                  <div key={k}>
                    <dt className="text-muted-foreground">{k}</dt>
                    <dd>{v}</dd>
                  </div>
                ))}
              </dl>
            </div>
          )}
        </div>
      )}

      {activeTab === "Preview" && (
        <div className="rounded-lg border p-4">
          {sandbox.preview_urls &&
          Object.keys(sandbox.preview_urls).length > 0 ? (
            <iframe
              src={Object.values(sandbox.preview_urls)[0]}
              className="h-[600px] w-full rounded border"
              title="Sandbox Preview"
            />
          ) : sandbox.vnc_url ? (
            <iframe
              src={sandbox.vnc_url}
              className="h-[600px] w-full rounded border"
              title="VNC Desktop"
            />
          ) : (
            <p className="text-sm text-muted-foreground">
              No preview available. Sandbox has no exposed ports.
            </p>
          )}
        </div>
      )}

      {activeTab === "Terminal" && token && sandbox.ws_url && (
        <div className="rounded-lg border overflow-hidden" style={{ height: "520px" }}>
          <SandboxTerminal wsUrl={sandbox.ws_url} token={token} />
        </div>
      )}
    </div>
  );
}
