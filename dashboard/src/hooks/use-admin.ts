"use client";

import { useQuery } from "@tanstack/react-query";
import { useApi } from "@/hooks/use-api";

export function useAdminSummary() {
  const api = useApi();

  return useQuery({
    queryKey: ["admin-summary"],
    queryFn: () => api!.getAdminSummary(),
    enabled: !!api,
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
  });
}

export function useAdminMetrics() {
  const api = useApi();

  return useQuery({
    queryKey: ["admin-metrics"],
    queryFn: () => api!.getAdminMetrics(),
    enabled: !!api,
    refetchInterval: 10_000,
    refetchIntervalInBackground: false,
  });
}

export function useAdminWarmPool() {
  const api = useApi();

  return useQuery({
    queryKey: ["admin-warm-pool"],
    queryFn: () => api!.getAdminWarmPool(),
    enabled: !!api,
    refetchInterval: 10_000,
    refetchIntervalInBackground: false,
  });
}

export function useAuditLogs(params?: {
  limit?: number;
  offset?: number;
  action?: string;
  subject?: string;
}) {
  const api = useApi();

  return useQuery({
    queryKey: ["audit-logs", params],
    queryFn: () => api!.getAuditLogs(params),
    enabled: !!api,
  });
}
