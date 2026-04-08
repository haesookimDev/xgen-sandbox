"use client";

import { useState } from "react";
import Link from "next/link";
import {
  useSandboxes,
  useCreateSandbox,
  useDeleteSandbox,
} from "@/hooks/use-sandboxes";
import { formatRelativeTime, formatTimeRemaining } from "@/lib/utils";

function StatusBadge({ status }: { status: string }) {
  const colors: Record<string, string> = {
    running: "bg-green-100 text-green-800",
    starting: "bg-yellow-100 text-yellow-800",
    stopping: "bg-orange-100 text-orange-800",
    stopped: "bg-gray-100 text-gray-800",
    error: "bg-red-100 text-red-800",
  };
  return (
    <span
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${colors[status] || "bg-gray-100 text-gray-800"}`}
    >
      {status}
    </span>
  );
}

function CreateDialog({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const [template, setTemplate] = useState("base");
  const [timeout, setTimeout] = useState(3600);
  const [ports, setPorts] = useState("");
  const [gui, setGui] = useState(false);
  const createMutation = useCreateSandbox();

  if (!open) return null;

  const handleCreate = async () => {
    await createMutation.mutateAsync({
      template,
      timeout_seconds: timeout,
      ports: ports
        ? ports.split(",").map((p) => parseInt(p.trim(), 10))
        : undefined,
      gui,
    });
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div className="w-full max-w-md rounded-lg border bg-card p-6 shadow-lg">
        <h2 className="mb-4 text-lg font-semibold">Create Sandbox</h2>

        <div className="space-y-4">
          <div>
            <label className="mb-1 block text-sm font-medium">Template</label>
            <select
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            >
              <option value="base">base</option>
              <option value="nodejs">nodejs</option>
              <option value="python">python</option>
              <option value="gui">gui</option>
            </select>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">
              Timeout (seconds)
            </label>
            <input
              type="number"
              value={timeout}
              onChange={(e) => setTimeout(Number(e.target.value))}
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium">
              Ports (comma separated)
            </label>
            <input
              type="text"
              value={ports}
              onChange={(e) => setPorts(e.target.value)}
              placeholder="3000, 8080"
              className="w-full rounded-md border bg-background px-3 py-2 text-sm"
            />
          </div>

          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="gui"
              checked={gui}
              onChange={(e) => setGui(e.target.checked)}
              className="rounded border"
            />
            <label htmlFor="gui" className="text-sm">
              Enable GUI (VNC)
            </label>
          </div>
        </div>

        <div className="mt-6 flex justify-end gap-3">
          <button
            onClick={onClose}
            className="rounded-md border px-4 py-2 text-sm hover:bg-accent"
          >
            Cancel
          </button>
          <button
            onClick={handleCreate}
            disabled={createMutation.isPending}
            className="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
          >
            {createMutation.isPending ? "Creating..." : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function SandboxesPage() {
  const { data: sandboxes, isLoading } = useSandboxes();
  const deleteMutation = useDeleteSandbox();
  const [showCreate, setShowCreate] = useState(false);
  const [filter, setFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");

  const filtered = sandboxes?.filter((sbx) => {
    if (filter && !sbx.id.includes(filter) && !sbx.template.includes(filter))
      return false;
    if (statusFilter && sbx.status !== statusFilter) return false;
    return true;
  });

  return (
    <div>
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold">Sandboxes</h1>
        <button
          onClick={() => setShowCreate(true)}
          className="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground hover:bg-primary/90"
        >
          Create Sandbox
        </button>
      </div>

      <div className="mb-4 flex gap-3">
        <input
          type="text"
          placeholder="Search by ID or template..."
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          className="rounded-md border bg-background px-3 py-2 text-sm"
        />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="rounded-md border bg-background px-3 py-2 text-sm"
        >
          <option value="">All Statuses</option>
          <option value="running">Running</option>
          <option value="starting">Starting</option>
          <option value="stopped">Stopped</option>
          <option value="error">Error</option>
        </select>
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : (
        <div className="overflow-hidden rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted">
              <tr>
                <th className="px-4 py-2 text-left font-medium">ID</th>
                <th className="px-4 py-2 text-left font-medium">Status</th>
                <th className="px-4 py-2 text-left font-medium">Template</th>
                <th className="px-4 py-2 text-left font-medium">Created</th>
                <th className="px-4 py-2 text-left font-medium">Expires</th>
                <th className="px-4 py-2 text-left font-medium">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered?.map((sbx) => (
                <tr key={sbx.id} className="border-t hover:bg-muted/50">
                  <td className="px-4 py-2 font-mono text-xs">
                    <Link
                      href={`/dashboard/sandboxes/${sbx.id}`}
                      className="text-primary hover:underline"
                    >
                      {sbx.id}
                    </Link>
                  </td>
                  <td className="px-4 py-2">
                    <StatusBadge status={sbx.status} />
                  </td>
                  <td className="px-4 py-2">{sbx.template}</td>
                  <td className="px-4 py-2 text-muted-foreground">
                    {formatRelativeTime(sbx.created_at)}
                  </td>
                  <td className="px-4 py-2 text-muted-foreground">
                    {formatTimeRemaining(sbx.expires_at)}
                  </td>
                  <td className="px-4 py-2">
                    <div className="flex gap-2">
                      <Link
                        href={`/dashboard/sandboxes/${sbx.id}`}
                        className="rounded border px-2 py-1 text-xs hover:bg-accent"
                      >
                        View
                      </Link>
                      <button
                        onClick={() => {
                          if (confirm(`Delete sandbox ${sbx.id}?`))
                            deleteMutation.mutate(sbx.id);
                        }}
                        className="rounded border border-destructive/30 px-2 py-1 text-xs text-destructive hover:bg-destructive/10"
                      >
                        Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {(!filtered || filtered.length === 0) && (
                <tr>
                  <td
                    colSpan={6}
                    className="px-4 py-8 text-center text-muted-foreground"
                  >
                    No sandboxes found
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}

      <CreateDialog open={showCreate} onClose={() => setShowCreate(false)} />
    </div>
  );
}
