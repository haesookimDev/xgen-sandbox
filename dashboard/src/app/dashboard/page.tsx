"use client";

import { useAdminSummary } from "@/hooks/use-admin";
import { useSandboxes } from "@/hooks/use-sandboxes";
import { StatusBadge } from "@/components/status-badge";
import { formatRelativeTime } from "@/lib/utils";

function StatCard({
  title,
  value,
  subtitle,
}: {
  title: string;
  value: string | number;
  subtitle?: string;
}) {
  return (
    <div className="rounded-lg border bg-card p-6">
      <p className="text-sm text-muted-foreground">{title}</p>
      <p className="mt-1 text-3xl font-bold">{value}</p>
      {subtitle && (
        <p className="mt-1 text-xs text-muted-foreground">{subtitle}</p>
      )}
    </div>
  );
}

export default function OverviewPage() {
  const { data: summary } = useAdminSummary();
  const { data: sandboxes } = useSandboxes();

  const warmPoolTotal = summary
    ? Object.values(summary.warm_pool).reduce((sum, p) => sum + p.available, 0)
    : 0;
  const warmPoolTarget = summary
    ? Object.values(summary.warm_pool).reduce((sum, p) => sum + p.target, 0)
    : 0;

  const recentSandboxes = sandboxes
    ?.sort(
      (a, b) =>
        new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    )
    .slice(0, 5);

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Overview</h1>

      <div className="mb-8 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Active Sandboxes"
          value={summary?.active_sandboxes ?? "-"}
        />
        <StatCard
          title="By Template"
          value={
            summary
              ? Object.entries(summary.sandboxes_by_template)
                  .map(([t, c]) => `${t}: ${c}`)
                  .join(", ") || "None"
              : "-"
          }
        />
        <StatCard
          title="Warm Pool"
          value={`${warmPoolTotal} / ${warmPoolTarget}`}
          subtitle="available / target"
        />
        <StatCard
          title="Statuses"
          value={
            summary
              ? Object.entries(summary.sandboxes_by_status)
                  .map(([s, c]) => `${s}: ${c}`)
                  .join(", ") || "None"
              : "-"
          }
        />
      </div>

      <div>
        <h2 className="mb-3 text-lg font-semibold">Recent Sandboxes</h2>
        {recentSandboxes && recentSandboxes.length > 0 ? (
          <div className="overflow-hidden rounded-lg border">
            <table className="w-full text-sm">
              <thead className="bg-muted">
                <tr>
                  <th className="px-4 py-2 text-left font-medium">ID</th>
                  <th className="px-4 py-2 text-left font-medium">Status</th>
                  <th className="px-4 py-2 text-left font-medium">Template</th>
                  <th className="px-4 py-2 text-left font-medium">Created</th>
                </tr>
              </thead>
              <tbody>
                {recentSandboxes.map((sbx) => (
                  <tr key={sbx.id} className="border-t">
                    <td className="px-4 py-2 font-mono text-xs">{sbx.id}</td>
                    <td className="px-4 py-2">
                      <StatusBadge status={sbx.status} />
                    </td>
                    <td className="px-4 py-2">{sbx.template}</td>
                    <td className="px-4 py-2 text-muted-foreground">
                      {formatRelativeTime(sbx.created_at)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">No sandboxes running</p>
        )}
      </div>
    </div>
  );
}
