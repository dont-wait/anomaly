import { toast } from "@/shared/notifications/toast";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { AUTH_STATUS } from "./authStatus";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { AuthContext, type AuthStatus } from "./authContext";
import {
  login as loginRequest,
  toLoginError,
  type LoginInput,
} from "./api/auth";
import { getInfo, type AuthUser } from "./api/profile";
import { ApiError } from "@/shared/lib/http";
import { defaultAuthTokenStore, type AuthTokenStore } from "./lib/token-store";

interface AuthProviderProps {
  children: ReactNode;
  tokenStore?: AuthTokenStore;
}

export const AuthProvider = ({
  children,
  tokenStore = defaultAuthTokenStore,
}: AuthProviderProps) => {
  const [status, setStatus] = useState<AuthStatus>(AUTH_STATUS.IDLE);
  const [user, setUser] = useState<AuthUser | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const logout = useCallback(() => {
    tokenStore.clearToken();
    if (!mountedRef.current) return;
    setToken(null);
    setUser(null);
    setStatus(AUTH_STATUS.UNAUTHENTICATED);
    setError(null);
  }, [tokenStore]);

  const refreshProfile = useCallback(
    async (options: { signal?: AbortSignal } = {}) => {
      const currentToken = tokenStore.getToken();
      if (!currentToken) {
        if (mountedRef.current) {
          setToken(null);
          setUser(null);
          setStatus(AUTH_STATUS.UNAUTHENTICATED);
        }
        return null;
      }

      try {
        const profile = await getInfo(currentToken, {
          signal: options.signal,
        });
        if (!mountedRef.current) return profile;
        setToken(currentToken);
        setUser(profile);
        setStatus(AUTH_STATUS.AUTHENTICATED);
        setError(null);
        return profile;
      } catch (requestError) {
        if (
          requestError instanceof ApiError &&
          (requestError.status === HTTP_STATUS.UNAUTHORIZED ||
            requestError.status === HTTP_STATUS.NOT_FOUND)
        ) {
          tokenStore.clearToken();
          if (!mountedRef.current) return null;
          setToken(null);
          setUser(null);
          setStatus(AUTH_STATUS.UNAUTHENTICATED);
          setError(null);
          return null;
        }
        const message = toLoginError(requestError);
        if (mountedRef.current) setError(message);
        throw requestError;
      }
    },
    [tokenStore],
  );

  const updateProfile = useCallback(async (input: {
    phone?: string;
    email?: string;
    address?: { line: string; provinceCode: string };
  }) => {
    const currentToken = tokenStore.getToken();
    if (!currentToken) throw new Error("Phiên đăng nhập đã hết hạn.");
    const { updateCurrentProfile } = await import("./api/profileMutations");
    await updateCurrentProfile(currentToken, input);
    const profile = await refreshProfile();
    if (!profile) throw new Error("Không thể tải lại hồ sơ sau khi cập nhật.");
    return profile;
  }, [refreshProfile, tokenStore]);

  const changePassword = useCallback(async (input: {
    currentPassword: string;
    newPassword: string;
  }) => {
    const currentToken = tokenStore.getToken();
    if (!currentToken) throw new Error("Phiên đăng nhập đã hết hạn.");
    const { changeCurrentPassword } = await import("./api/profileMutations");
    await changeCurrentPassword(currentToken, input);
  }, [tokenStore]);

  const uploadProfileAvatar = useCallback(async (file: File) => {
    const currentToken = tokenStore.getToken();
    if (!currentToken) throw new Error("Phiên đăng nhập đã hết hạn.");
    const { uploadAvatar } = await import("./api/profileMutations");
    await uploadAvatar(currentToken, file);
    return refreshProfile();
  }, [refreshProfile, tokenStore]);

  const uploadProfileDocument = useCallback(async (type: string, file: File) => {
    const currentToken = tokenStore.getToken();
    if (!currentToken) throw new Error("Phiên đăng nhập đã hết hạn.");
    const { uploadDocument } = await import("./api/profileMutations");
    await uploadDocument(currentToken, type, file);
    return refreshProfile();
  }, [refreshProfile, tokenStore]);

  useEffect(() => {
    const controller = new AbortController();
    let cancelled = false;

    const restoreSession = async () => {
      const storedToken = tokenStore.getToken();
      if (!storedToken) {
        setStatus(AUTH_STATUS.UNAUTHENTICATED);
        return;
      }

      setStatus(AUTH_STATUS.RESTORING);
      setToken(storedToken);
      try {
        const profile = await getInfo(storedToken, {
          signal: controller.signal,
        });
        if (cancelled) return;
        setUser(profile);
        setStatus(AUTH_STATUS.AUTHENTICATED);
        setError(null);
      } catch (requestError) {
        if (cancelled) return;
        if (
          requestError instanceof ApiError &&
          (requestError.status === HTTP_STATUS.UNAUTHORIZED ||
            requestError.status === HTTP_STATUS.NOT_FOUND)
        ) {
          tokenStore.clearToken();
          setToken(null);
          setUser(null);
          setStatus(AUTH_STATUS.UNAUTHENTICATED);
          setError(null);
          return;
        }
        setStatus(AUTH_STATUS.UNAUTHENTICATED);
        setError(toLoginError(requestError));
        toast.error(toLoginError(requestError));
      }
    };

    void restoreSession();
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [tokenStore]);

  const login = useCallback(
    async (input: LoginInput, options: { signal?: AbortSignal } = {}) => {
      setError(null);
      try {
        const response = await loginRequest(input, {
          signal: options.signal,
        });
        tokenStore.setToken(response.token);
        if (!mountedRef.current) return response.user;
        setToken(response.token);
        setUser(response.user);
        setStatus(AUTH_STATUS.AUTHENTICATED);
        return response.user;
      } catch (requestError) {
        const message = toLoginError(requestError);
        if (mountedRef.current) setError(message);
        throw requestError;
      }
    },
    [tokenStore],
  );

  const value = useMemo(
    () => ({
      status,
      user,
      token,
      error,
      login,
      logout,
      refreshProfile,
      updateProfile,
      changePassword,
      uploadProfileAvatar,
      uploadProfileDocument,
    }),
    [status, user, token, error, login, logout, refreshProfile, updateProfile, changePassword, uploadProfileAvatar, uploadProfileDocument],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};
