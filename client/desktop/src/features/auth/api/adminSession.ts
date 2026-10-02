export interface AdminSession {
  accessToken: string;
  expiresAt: string;
}

interface AccountProfile {
  role: string;
}

interface ApiSuccess<T> {
  data: T;
}

export const API_BASE_URL =
  import.meta.env.VITE_API_ENDPOINT || "http://localhost:8080";

const SESSION_STORAGE_KEY = "anomaly.admin.session";
let inFlightVerification: Promise<boolean> | null = null;

export function saveAdminSession(session: AdminSession) {
  window.localStorage.setItem(SESSION_STORAGE_KEY, JSON.stringify(session));
}

export function clearAdminSession() {
  window.localStorage.removeItem(SESSION_STORAGE_KEY);
}

export function readAdminSession(): AdminSession | null {
  const stored = window.localStorage.getItem(SESSION_STORAGE_KEY);
  if (!stored) return null;

  try {
    const session = JSON.parse(stored) as Partial<AdminSession>;
    if (
      typeof session.accessToken !== "string" ||
      typeof session.expiresAt !== "string" ||
      !Number.isFinite(Date.parse(session.expiresAt)) ||
      Date.parse(session.expiresAt) <= Date.now()
    ) {
      clearAdminSession();
      return null;
    }
    return session as AdminSession;
  } catch {
    clearAdminSession();
    return null;
  }
}

async function verifyStoredAdminSession(): Promise<boolean> {
  const session = readAdminSession();
  if (!session) return false;

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/auth/me`, {
      headers: { Authorization: `Bearer ${session.accessToken}` },
    });
  } catch {
    return false;
  }

  if (!response.ok) {
    if (
      response.status === 401 ||
      response.status === 403 ||
      response.status === 404
    ) {
      clearAdminSession();
    }
    return false;
  }
  try {
    const body = (await response.json()) as ApiSuccess<AccountProfile>;
    if (body.data?.role === "admin") return true;
  } catch {
    clearAdminSession();
    return false;
  }
  clearAdminSession();
  return false;
}

export function verifyAdminSession(): Promise<boolean> {
  if (inFlightVerification) return inFlightVerification;

  inFlightVerification = verifyStoredAdminSession().finally(() => {
    inFlightVerification = null;
  });
  return inFlightVerification;
}
