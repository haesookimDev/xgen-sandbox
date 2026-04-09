"use client";

import { useMemo } from "react";
import { useAuth } from "@/providers/auth-provider";
import { createApi } from "@/lib/api";

export function useApi() {
  const { token } = useAuth();
  return useMemo(() => (token ? createApi(token) : null), [token]);
}
