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
  changeCurrentPassword,
  confirmEmailChange as verifyEmailChange,
  startEmailChange as requestEmailChange,
  updateCurrentProfile,
  uploadAvatar,
  uploadDocument,
  type ProfileUpdateInput,
  type UploadProgress,
} from "./api/profileMutations";
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
  // Chỉ response của lần đọc hồ sơ mới nhất được phép cập nhật state.
  const profileGenerationRef = useRef(0);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const logout = useCallback(() => {
    profileGenerationRef.current += 1;
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

      const generation = ++profileGenerationRef.current;
      try {
        const profile = await getInfo(currentToken, {
          signal: options.signal,
        });
        if (!mountedRef.current) return profile;
        if (generation !== profileGenerationRef.current) return profile;
        setToken(currentToken);
        setUser(profile);
        setStatus(AUTH_STATUS.AUTHENTICATED);
        setError(null);
        return profile;
      } catch (requestError) {
        if (generation !== profileGenerationRef.current) return null;
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

  const requireToken = useCallback(() => {
    const currentToken = tokenStore.getToken();
    if (!currentToken) throw new Error("Phiên đăng nhập đã hết hạn.");
    return currentToken;
  }, [tokenStore]);

  // Thay đổi đã được server xác nhận: lỗi khi tải lại không được biến nó thành thất bại.
  const refreshAfterMutation = useCallback(
    () => refreshProfile().catch(() => null),
    [refreshProfile],
  );

  // Áp dụng kết quả đã được server xác nhận, trừ khi phiên đã đổi trong lúc chờ:
  // đăng xuất giữa chừng không được đưa dữ liệu hồ sơ trở lại state.
  const applyConfirmed = useCallback(
    (
      requestToken: string,
      update: (user: AuthUser | null) => AuthUser | null,
    ) => {
      if (tokenStore.getToken() !== requestToken) return false;
      profileGenerationRef.current += 1;
      if (mountedRef.current) setUser(update);
      return true;
    },
    [tokenStore],
  );

  const updateProfile = useCallback(
    async (input: ProfileUpdateInput) => {
      const requestToken = requireToken();
      const updated = await updateCurrentProfile(requestToken, input);
      if (!applyConfirmed(requestToken, () => updated)) return updated;
      return (await refreshAfterMutation()) ?? updated;
    },
    [applyConfirmed, refreshAfterMutation, requireToken],
  );

  const changePassword = useCallback(
    async (input: { currentPassword: string; newPassword: string }) => {
      await changeCurrentPassword(requireToken(), input);
      logout();
      toast.success("Đã đổi mật khẩu. Vui lòng đăng nhập lại.");
    },
    [logout, requireToken],
  );

  const startEmailChange = useCallback(
    async (newEmail: string) => {
      const requestToken = requireToken();
      await requestEmailChange(requestToken, newEmail);
      // OTP đã gửi: màn nhập mã phải hiện ngay cả khi tải lại hồ sơ thất bại.
      if (
        !applyConfirmed(requestToken, (current) =>
          current ? { ...current, pendingEmail: newEmail } : current,
        )
      ) {
        return null;
      }
      return refreshAfterMutation();
    },
    [applyConfirmed, refreshAfterMutation, requireToken],
  );

  const confirmEmailChange = useCallback(
    async (input: { currentEmailOtp: string; newEmailOtp: string }) => {
      const requestToken = requireToken();
      await verifyEmailChange(requestToken, input);
      if (
        !applyConfirmed(requestToken, (current) =>
          current
            ? {
                ...current,
                email: current.pendingEmail ?? current.email,
                pendingEmail: null,
              }
            : current,
        )
      ) {
        return null;
      }
      return refreshAfterMutation();
    },
    [applyConfirmed, refreshAfterMutation, requireToken],
  );

  const uploadProfileAvatar = useCallback(
    async (file: File, onProgress?: UploadProgress) => {
      await uploadAvatar(requireToken(), file, onProgress);
      return refreshAfterMutation();
    },
    [refreshAfterMutation, requireToken],
  );

  const uploadProfileDocument = useCallback(
    async (type: string, file: File, onProgress?: UploadProgress) => {
      await uploadDocument(requireToken(), type, file, onProgress);
      return refreshAfterMutation();
    },
    [refreshAfterMutation, requireToken],
  );

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
      const generation = ++profileGenerationRef.current;
      try {
        const profile = await getInfo(storedToken, {
          signal: controller.signal,
        });
        if (cancelled || generation !== profileGenerationRef.current) return;
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
        profileGenerationRef.current += 1;
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
      startEmailChange,
      confirmEmailChange,
      uploadProfileAvatar,
      uploadProfileDocument,
    }),
    [
      status,
      user,
      token,
      error,
      login,
      logout,
      refreshProfile,
      updateProfile,
      changePassword,
      startEmailChange,
      confirmEmailChange,
      uploadProfileAvatar,
      uploadProfileDocument,
    ],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};
