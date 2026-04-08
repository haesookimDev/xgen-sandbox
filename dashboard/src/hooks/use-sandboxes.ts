"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "@/providers/auth-provider";
import { createApi, type CreateSandboxRequest } from "@/lib/api";

export function useSandboxes() {
  const { token } = useAuth();
  const api = token ? createApi(token) : null;

  return useQuery({
    queryKey: ["sandboxes"],
    queryFn: () => api!.listSandboxes(),
    enabled: !!api,
    refetchInterval: 5_000,
  });
}

export function useSandbox(id: string) {
  const { token } = useAuth();
  const api = token ? createApi(token) : null;

  return useQuery({
    queryKey: ["sandbox", id],
    queryFn: () => api!.getSandbox(id),
    enabled: !!api && !!id,
    refetchInterval: 5_000,
  });
}

export function useCreateSandbox() {
  const { token } = useAuth();
  const queryClient = useQueryClient();
  const api = token ? createApi(token) : null;

  return useMutation({
    mutationFn: (req: CreateSandboxRequest) => api!.createSandbox(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sandboxes"] });
      queryClient.invalidateQueries({ queryKey: ["admin-summary"] });
    },
  });
}

export function useDeleteSandbox() {
  const { token } = useAuth();
  const queryClient = useQueryClient();
  const api = token ? createApi(token) : null;

  return useMutation({
    mutationFn: (id: string) => api!.deleteSandbox(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sandboxes"] });
      queryClient.invalidateQueries({ queryKey: ["admin-summary"] });
    },
  });
}
