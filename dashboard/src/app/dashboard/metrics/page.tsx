"use client";

import { useState, useEffect } from "react";
import { useAdminMetrics, useAdminWarmPool } from "@/hooks/use-admin";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

interface MetricsDataPoint {
  time: string;
  activeSandboxes: number;
}

export default function MetricsPage() {
  const { data: metrics } = useAdminMetrics();
  const { data: warmPool } = useAdminWarmPool();
  const [history, setHistory] = useState<MetricsDataPoint[]>([]);

  useEffect(() => {
    if (metrics) {
      setHistory((prev) => {
        const next = [
          ...prev,
          {
            time: new Date().toLocaleTimeString(),
            activeSandboxes: metrics.active_sandboxes,
          },
        ];
        // Keep last 60 data points (~10 minutes at 10s interval)
        return next.slice(-60);
      });
    }
  }, [metrics]);

  return (
    <div>
      <h1 className="mb-6 text-2xl font-bold">Metrics</h1>

      <div className="mb-8 rounded-lg border p-6">
        <h2 className="mb-4 text-lg font-semibold">Active Sandboxes</h2>
        <div className="h-64">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={history}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="time" tick={{ fontSize: 11 }} />
              <YAxis allowDecimals={false} />
              <Tooltip />
              <Line
                type="monotone"
                dataKey="activeSandboxes"
                stroke="#2563eb"
                strokeWidth={2}
                dot={false}
              />
            </LineChart>
          </ResponsiveContainer>
        </div>
      </div>

      <div className="rounded-lg border p-6">
        <h2 className="mb-4 text-lg font-semibold">Warm Pool Status</h2>
        {warmPool?.pools && warmPool.pools.length > 0 ? (
          <div className="space-y-4">
            {warmPool.pools.map((pool) => {
              const pct =
                pool.target > 0
                  ? Math.round((pool.available / pool.target) * 100)
                  : 0;
              const barColor =
                pct >= 80
                  ? "bg-green-500"
                  : pct >= 40
                    ? "bg-yellow-500"
                    : "bg-red-500";

              return (
                <div key={pool.template}>
                  <div className="mb-1 flex items-center justify-between text-sm">
                    <span className="font-medium">{pool.template}</span>
                    <span className="text-muted-foreground">
                      {pool.available} / {pool.target}
                    </span>
                  </div>
                  <div className="h-3 w-full overflow-hidden rounded-full bg-muted">
                    <div
                      className={`h-full rounded-full transition-all ${barColor}`}
                      style={{ width: `${Math.min(pct, 100)}%` }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            Warm pool is not configured
          </p>
        )}
      </div>
    </div>
  );
}
