import { requestJson } from "@/shared/lib/http";

export interface AuthUser {
  id: string;
  accountNo: string;
  username: string;
  fullName?: string;
  email: string;
  currency: string;
  idCardFrontUrl: string;
  idCardBackUrl: string;
  liveVideoUrl: string;
  isVerify: boolean;
  amount: number;
}

export async function getInfo(
  token: string,
  options: { signal?: AbortSignal } = {},
): Promise<AuthUser> {
  if (!token) {
    throw new Error("Thiếu token xác thực.");
  }
  return requestJson<AuthUser>("/api/auth/me", {
    token,
    signal: options.signal,
  });
}