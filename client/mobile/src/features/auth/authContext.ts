import { createContext } from "react";
import type { AuthUser, LoginInput } from "@/features/auth/api/auth";
import type { AuthStatus } from "./authStatus";

export type { AuthStatus } from "./authStatus";

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
