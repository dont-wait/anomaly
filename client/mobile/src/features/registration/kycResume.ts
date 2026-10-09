import type { AuthUser } from "@/features/auth/api";

export const KYC_RESUME_STORAGE_KEY = "anomaly.kyc.resume";

export interface KycResumeProfile {
  name: string;
  id: string;
  dob: string;
  issuedDate: string;
  email: string;
}

export interface KycResumeSession {
  kycToken: string;
  kycExpiresAt: string;
  user: AuthUser;
  profile?: KycResumeProfile;
}

function isSession(value: unknown): value is KycResumeSession {
  if (!value || typeof value !== "object") return false;
  const session = value as KycResumeSession;
  const profile = session.profile;
  return (
    typeof session.kycToken === "string" &&
    session.kycToken.length > 0 &&
    typeof session.kycExpiresAt === "string" &&
    Number.isFinite(Date.parse(session.kycExpiresAt)) &&
    !!session.user &&
    typeof session.user.id === "string" &&
    session.user.id.length > 0 &&
    session.user.isVerify === false &&
    (profile === undefined ||
      (typeof profile.name === "string" &&
        typeof profile.id === "string" &&
        typeof profile.dob === "string" &&
        typeof profile.issuedDate === "string" &&
        typeof profile.email === "string"))
  );
}

export function saveKycResumeSession(session: KycResumeSession): void {
  sessionStorage.setItem(KYC_RESUME_STORAGE_KEY, JSON.stringify(session));
}

export function clearKycResumeSession(): void {
  sessionStorage.removeItem(KYC_RESUME_STORAGE_KEY);
}

export function restoreKycResumeSession(): {
  session: KycResumeSession | null;
  expired: boolean;
} {
  const raw = sessionStorage.getItem(KYC_RESUME_STORAGE_KEY);
  if (!raw) return { session: null, expired: false };
  try {
    const session: unknown = JSON.parse(raw);
    if (!isSession(session)) {
      clearKycResumeSession();
      return { session: null, expired: false };
    }
    if (Date.parse(session.kycExpiresAt) <= Date.now()) {
      clearKycResumeSession();
      return { session: null, expired: true };
    }
    return { session, expired: false };
  } catch {
    clearKycResumeSession();
    return { session: null, expired: false };
  }
}
