import { createContext } from "react";
import type { AuthUser, LoginInput } from "@/features/auth/api/auth";
import type {
  ProfileUpdateInput,
  UploadProgress,
} from "@/features/auth/api/profileMutations";
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
  updateProfile: (input: ProfileUpdateInput) => Promise<AuthUser>;
  changePassword: (input: {
    currentPassword: string;
    newPassword: string;
  }) => Promise<void>;
  startEmailChange: (newEmail: string) => Promise<AuthUser | null>;
  confirmEmailChange: (input: {
    currentEmailOtp: string;
    newEmailOtp: string;
  }) => Promise<AuthUser | null>;
  uploadProfileAvatar: (
    file: File,
    onProgress?: UploadProgress,
  ) => Promise<AuthUser | null>;
  uploadProfileDocument: (
    type: string,
    file: File,
    onProgress?: UploadProgress,
  ) => Promise<AuthUser | null>;
}

export const AuthContext = createContext<AuthContextValue | undefined>(
  undefined,
);
