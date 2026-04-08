"use client";

import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/providers/auth-provider";
import { createApi } from "@/lib/api";

export function useAdminSummary() {
  const { token } = useAuth();
  const api = token ? createApi(token) : null;

  return useQuery({
    queryKey: ["admin-summary"],
    queryFn: () => api!.getAdminSummary(),
    enabled: !!api,
    refetchInterval: 5_000,
  });
}

export function useAdminMetrics() {
  const { token } = useAuth();
  const api = token ? createApi(token) : null;

  return useQuery({
    queryKey: ["admin-metrics"],
    queryFn: () => api!.getAdminMetrics(),
    enabled: !!api,
    refetchInterval: 10_000,
  });
}

export function useAdminWarmPool() {
  const { token } = useAuth();
  const api = token ? createApi(token) : null;

  return useQuery({
    queryKey: ["admin-warm-pool"],
    queryFn: () => api!.getAdminWarmPool(),
    enabled: !!api,
    refetchInterval: 10_000,
  });
}

export function useAuditLogs(params?: {
  limit?: number;
  offset?: number;
  action?: string;
  subject?: string;
}) {
  const { token } = useAuth();
  const api = token ? createApi(token) : null;

  return useQuery({
    queryKey: ["audit-logs", params],
    queryFn: () => api!.getAuditLogs(params),
    enabled: !!api,
  });
}
