import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { ACCOUNT_ROLE } from "./accountRole";

export interface AdminSession {
  accessToken: string;
  expiresAt: string;
}

export interface AdminProfile {
  id: string;
  accountNo: string;
  username: string;
  fullName: string;
  email: string;
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

export type AdminSessionVerification =
  | { status: "valid"; profile: AdminProfile }
  | { status: "invalid" | "unavailable" };

function isAdminProfile(value: unknown): value is AdminProfile {
  if (!value || typeof value !== "object") return false;
  const profile = value as Partial<AdminProfile>;
  return (
    typeof profile.id === "string" &&
    profile.id !== "" &&
    typeof profile.accountNo === "string" &&
    profile.accountNo !== "" &&
    typeof profile.username === "string" &&
    profile.username !== "" &&
    typeof profile.fullName === "string" &&
    profile.fullName !== "" &&
    typeof profile.email === "string" &&
    profile.email !== "" &&
    typeof profile.role === "string"
  );
}

async function verifyStoredAdminSession(): Promise<AdminSessionVerification> {
  const session = readAdminSession();
  if (!session) return { status: "invalid" };

  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}/api/auth/me`, {
      headers: { Authorization: `Bearer ${session.accessToken}` },
    });
  } catch {
    return { status: "unavailable" };
  }

  if (!response.ok) {
    if (
      response.status >= HTTP_STATUS.INTERNAL_SERVER_ERROR ||
      response.status === HTTP_STATUS.TOO_MANY_REQUESTS
    ) {
      return { status: "unavailable" };
    }
    clearAdminSession();
    return { status: "invalid" };
  }
  try {
    const body = (await response.json()) as ApiSuccess<unknown>;
    if (!isAdminProfile(body.data)) {
      clearAdminSession();
      return { status: "invalid" };
    }
    if (body.data.role === ACCOUNT_ROLE.ADMIN) {
      return { status: "valid", profile: body.data };
    }
  } catch {
    clearAdminSession();
    return { status: "invalid" };
  }
  clearAdminSession();
  return { status: "invalid" };
}

export function verifyAdminSession(): Promise<AdminSessionVerification> {
  if (inFlightVerification) return inFlightVerification;

  inFlightVerification = verifyStoredAdminSession().finally(() => {
    inFlightVerification = null;
  });
  return inFlightVerification;
}
