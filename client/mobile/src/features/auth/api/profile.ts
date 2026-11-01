import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { requestApi } from "@/shared/lib/http";

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
  phone?: string;
  dateOfBirth?: string;
  address?: { line?: string; provinceCode?: string };
  avatar?: ProfileMedia;
  documents?: ProfileMedia[];
  identity?: {
    type?: string;
    number?: string;
    issuedDate?: string;
    issuedPlace?: string;
    permanentAddress?: string;
  };
}

export interface ProfileMedia {
  mediaId: string;
  mediaUrl: string;
  type: string;
  fileName: string;
  contentType: string;
  sizeBytes: number;
  uploadedAt: string;
  status: string;
}

export async function getInfo(
  token: string,
  options: { signal?: AbortSignal } = {},
): Promise<AuthUser> {
  if (!token) {
    throw new Error("Thiếu token xác thực.");
  }
  return requestApi<AuthUser>(API_ENDPOINTS.AUTH.ME, {
    token,
    signal: options.signal,
  });
}
