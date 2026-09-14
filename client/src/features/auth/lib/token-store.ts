export interface AuthTokenStore {
  getToken(): string | null;
  setToken(token: string): void;
  clearToken(): void;
}

export const AUTH_TOKEN_STORAGE_KEY = "anomaly.auth.token";

function storage(): Storage | null {
  try {
    return globalThis.localStorage;
  } catch {
    return null;
  }
}

class LocalStorageAuthTokenStore implements AuthTokenStore {
  constructor(private readonly key: string = AUTH_TOKEN_STORAGE_KEY) {}

  getToken(): string | null {
    const currentStorage = storage();
    if (!currentStorage) return null;
    try {
      return currentStorage.getItem(this.key);
    } catch {
      return null;
    }
  }

  setToken(token: string): void {
    if (!token) {
      this.clearToken();
      return;
    }
    try {
      storage()?.setItem(this.key, token);
    } catch {
      // Storage unavailable; token stays in memory only.
    }
  }

  clearToken(): void {
    try {
      storage()?.removeItem(this.key);
    } catch {
      // Ignore storage errors on logout.
    }
  }
}

export function createLocalStorageAuthTokenStore(
  key: string = AUTH_TOKEN_STORAGE_KEY,
): AuthTokenStore {
  return new LocalStorageAuthTokenStore(key);
}

export const defaultAuthTokenStore: AuthTokenStore =
  createLocalStorageAuthTokenStore();
