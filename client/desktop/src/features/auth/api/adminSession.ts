import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { ACCOUNT_ROLE } from "./accountRole";

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
let inFlightVerification: Promise<AdminSessionVerification> | null = null;

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

export type AdminSessionVerification = "valid" | "invalid" | "unavailable";

async function verifyStoredAdminSession(): Promise<AdminSessionVerification> {
  const session = readAdminSession();
  if (!session) return "invalid";

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/auth/me`, {
      headers: { Authorization: `Bearer ${session.accessToken}` },
    });
  } catch {
    return "unavailable";
  }

  if (!response.ok) {
    if (
      response.status === HTTP_STATUS.UNAUTHORIZED ||
      response.status === HTTP_STATUS.FORBIDDEN ||
      response.status === HTTP_STATUS.NOT_FOUND
    ) {
      clearAdminSession();
    }
    return response.status >= HTTP_STATUS.INTERNAL_SERVER_ERROR ||
      response.status === HTTP_STATUS.TOO_MANY_REQUESTS
      ? "unavailable"
      : "invalid";
  }
  try {
    const body = (await response.json()) as ApiSuccess<AccountProfile>;
    if (body.data?.role === ACCOUNT_ROLE.ADMIN) return "valid";
  } catch {
    return "unavailable";
  }
  clearAdminSession();
  return "invalid";
}

export function verifyAdminSession(): Promise<AdminSessionVerification> {
  if (inFlightVerification) return inFlightVerification;

  inFlightVerification = verifyStoredAdminSession().finally(() => {
    inFlightVerification = null;
  });
  return inFlightVerification;
}
