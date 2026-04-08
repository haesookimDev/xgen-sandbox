"use client";

import { useState } from "react";
import { useAuditLogs } from "@/hooks/use-admin";

export default function LogsPage() {
  const [offset, setOffset] = useState(0);
  const [actionFilter, setActionFilter] = useState("");
  const [subjectFilter, setSubjectFilter] = useState("");
  const limit = 50;

  const { data, isLoading } = useAuditLogs({
    limit,
    offset,
    action: actionFilter || undefined,
    subject: subjectFilter || undefined,
  });

  const statusColor = (status: number) => {
    if (status >= 500) return "text-red-600";
    if (status >= 400) return "text-yellow-600";
    return "text-green-600";
  };

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Audit Logs</h1>

      <div className="mb-4 flex gap-3">
        <input
          type="text"
          placeholder="Filter by action (POST, DELETE...)"
          value={actionFilter}
          onChange={(e) => {
            setActionFilter(e.target.value);
            setOffset(0);
          }}
          className="rounded-md border bg-background px-3 py-2 text-sm"
        />
        <input
          type="text"
          placeholder="Filter by subject"
          value={subjectFilter}
          onChange={(e) => {
            setSubjectFilter(e.target.value);
            setOffset(0);
          }}
          className="rounded-md border bg-background px-3 py-2 text-sm"
        />
      </div>

      {isLoading ? (
        <p className="text-sm text-muted-foreground">Loading...</p>
      ) : (
        <>
          <div className="overflow-hidden rounded-lg border">
            <table className="w-full text-sm">
              <thead className="bg-muted">
                <tr>
                  <th className="px-4 py-2 text-left font-medium">
                    Timestamp
                  </th>
                  <th className="px-4 py-2 text-left font-medium">Subject</th>
                  <th className="px-4 py-2 text-left font-medium">Role</th>
                  <th className="px-4 py-2 text-left font-medium">Action</th>
                  <th className="px-4 py-2 text-left font-medium">Status</th>
                  <th className="px-4 py-2 text-left font-medium">
                    Remote IP
                  </th>
                </tr>
              </thead>
              <tbody>
                {data?.entries?.map((entry, i) => (
                  <tr key={i} className="border-t hover:bg-muted/50">
                    <td className="px-4 py-2 text-xs text-muted-foreground">
                      {new Date(entry.timestamp).toLocaleString()}
                    </td>
                    <td className="px-4 py-2">{entry.subject}</td>
                    <td className="px-4 py-2">
                      {entry.role && (
                        <span className="rounded bg-muted px-1.5 py-0.5 text-xs">
                          {entry.role}
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-2 font-mono text-xs">
                      {entry.action}
                    </td>
                    <td
                      className={`px-4 py-2 font-medium ${statusColor(entry.status)}`}
                    >
                      {entry.status}
                    </td>
                    <td className="px-4 py-2 text-xs text-muted-foreground">
                      {entry.remote_ip}
                    </td>
                  </tr>
                ))}
                {(!data?.entries || data.entries.length === 0) && (
                  <tr>
                    <td
                      colSpan={6}
                      className="px-4 py-8 text-center text-muted-foreground"
                    >
                      No audit log entries found
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </div>

          {data && data.total > limit && (
            <div className="mt-4 flex items-center justify-between">
              <p className="text-sm text-muted-foreground">
                Showing {offset + 1}-
                {Math.min(offset + limit, data.total)} of {data.total}
              </p>
              <div className="flex gap-2">
                <button
                  onClick={() => setOffset(Math.max(0, offset - limit))}
                  disabled={offset === 0}
                  className="rounded-md border px-3 py-1 text-sm disabled:opacity-50"
                >
                  Previous
                </button>
                <button
                  onClick={() => setOffset(offset + limit)}
                  disabled={offset + limit >= data.total}
                  className="rounded-md border px-3 py-1 text-sm disabled:opacity-50"
                >
                  Next
                </button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  );
}
