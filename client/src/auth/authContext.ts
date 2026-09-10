import { createContext } from "react";
import type { AuthUser, LoginInput } from "@/api/auth";

export type AuthStatus =
  "idle" | "restoring" | "authenticated" | "unauthenticated";

export interface AuthState {
  status: AuthStatus;
  user: AuthUser | null;
  token: string | null;
  error: string | null;
}

export interface AuthContextValue extends AuthState {
  login: (
    input: LoginInput,
    options?: { signal?: AbortSignal },
  ) => Promise<AuthUser>;
  logout: () => void;
  refreshProfile: (options?: {
    signal?: AbortSignal;
  }) => Promise<AuthUser | null>;
}

export const AuthContext = createContext<AuthContextValue | undefined>(
  undefined,
);
