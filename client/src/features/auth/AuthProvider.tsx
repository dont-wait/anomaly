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
    getCurrentUser,
    login as loginRequest,
    toLoginError,
    type AuthUser,
    type LoginInput,
} from "./api/auth";
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
    const [status, setStatus] = useState<AuthStatus>("idle");
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
        setStatus("unauthenticated");
        setError(null);
    }, [tokenStore]);

    const refreshProfile = useCallback(
        async (options: { signal?: AbortSignal } = {}) => {
            const currentToken = tokenStore.getToken();
            if (!currentToken) {
                if (mountedRef.current) {
                    setToken(null);
                    setUser(null);
                    setStatus("unauthenticated");
                }
                return null;
            }

            try {
                const profile = await getCurrentUser(currentToken, {
                    signal: options.signal,
                });
                if (!mountedRef.current) return profile;
                setToken(currentToken);
                setUser(profile);
                setStatus("authenticated");
                setError(null);
                return profile;
            } catch (requestError) {
                if (
                    requestError instanceof ApiError &&
                    (requestError.status === 401 ||
                        requestError.status === 404)
                ) {
                    tokenStore.clearToken();
                    if (!mountedRef.current) return null;
                    setToken(null);
                    setUser(null);
                    setStatus("unauthenticated");
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

    useEffect(() => {
        const controller = new AbortController();
        let cancelled = false;

        const restoreSession = async () => {
            const storedToken = tokenStore.getToken();
            if (!storedToken) {
                setStatus("unauthenticated");
                return;
            }

            setStatus("restoring");
            setToken(storedToken);
            try {
                const profile = await getCurrentUser(storedToken, {
                    signal: controller.signal,
                });
                if (cancelled) return;
                setUser(profile);
                setStatus("authenticated");
                setError(null);
            } catch (requestError) {
                if (cancelled) return;
                if (
                    requestError instanceof ApiError &&
                    (requestError.status === 401 ||
                        requestError.status === 404)
                ) {
                    tokenStore.clearToken();
                    setToken(null);
                    setUser(null);
                    setStatus("unauthenticated");
                    setError(null);
                    return;
                }
                setStatus("unauthenticated");
                setError(toLoginError(requestError));
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
                setStatus("authenticated");
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
        }),
        [status, user, token, error, login, logout, refreshProfile],
    );

    return (
        <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
    );
};
