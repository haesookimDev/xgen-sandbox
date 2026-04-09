"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
} from "react";
import { exchangeToken } from "@/lib/api";
import {
  getStoredToken,
  storeToken,
  clearToken,
} from "@/lib/auth";

interface AuthContextValue {
  token: string | null;
  isAuthenticated: boolean;
  login: (apiKey: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue>({
  token: null,
  isAuthenticated: false,
  login: async () => {},
  logout: () => {},
});

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);

  useEffect(() => {
    const stored = getStoredToken();
    if (stored) setToken(stored);
  }, []);

  const login = useCallback(async (apiKey: string) => {
    const res = await exchangeToken(apiKey);
    storeToken(res.token);
    setToken(res.token);
  }, []);

  const logout = useCallback(() => {
    clearToken();
    setToken(null);
  }, []);

  return (
    <AuthContext.Provider
      value={{ token, isAuthenticated: !!token, login, logout }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  return useContext(AuthContext);
}
