"use client";

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "@/hooks/use-api";
import { type CreateSandboxRequest } from "@/lib/api";

export function useSandboxes() {
  const api = useApi();

  return useQuery({
    queryKey: ["sandboxes"],
    queryFn: () => api!.listSandboxes(),
    enabled: !!api,
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
  });
}

export function useSandbox(id: string) {
  const api = useApi();

  return useQuery({
    queryKey: ["sandbox", id],
    queryFn: () => api!.getSandbox(id),
    enabled: !!api && !!id,
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
  });
}

export function useCreateSandbox() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: CreateSandboxRequest) => api!.createSandbox(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sandboxes"] });
      queryClient.invalidateQueries({ queryKey: ["admin-summary"] });
    },
  });
}

export function useDeleteSandbox() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (id: string) => api!.deleteSandbox(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["sandboxes"] });
      queryClient.invalidateQueries({ queryKey: ["admin-summary"] });
    },
  });
}
