import { API_BASE_URL, readAdminSession } from "@/features/auth/api/adminSession";
import { LoansApiError, request } from "@/features/loans/api/loansApi";

export { LoansApiError as KycReviewApiError } from "@/features/loans/api/loansApi";

const ROOT = "/api/admin/kyc/reviews";

export interface KycReviewIdentity {
  type: string;
  number: string;
  fullName: string;
  dateOfBirth?: string;
  issuedDate?: string;
  issuedPlace?: string;
  permanentAddress?: string;
}

export interface KycReviewMedia {
  front: string;
  back: string;
  video: string;
  faceImage?: string;
}

export interface KycReviewItem {
  accountId: string;
  username: string;
  email: string;
  phone: string;
  sessionId: string;
  attemptNo: number;
  submittedAt: string;
  declared: KycReviewIdentity;
  registration: KycReviewIdentity;
  media: KycReviewMedia;
}

export type KycReviewDecision = "APPROVED" | "REJECTED";

export async function listKycReviews(): Promise<KycReviewItem[]> {
  const body = await request<{ data: KycReviewItem[] }>(ROOT);
  return body.data;
}

export async function decideKycReview(
  accountId: string,
  input: { sessionId: string; decision: KycReviewDecision; reason?: string },
): Promise<void> {
  await request(`${ROOT}/${encodeURIComponent(accountId)}/decision`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

/** Loads one evidence object with the admin bearer token. */
export async function downloadKycMedia(path: string): Promise<Blob> {
  const session = readAdminSession();
  if (!session) {
    throw new LoansApiError("Phiên đăng nhập admin đã hết hạn.", 401);
  }
  const controller = new AbortController();
  const timer = window.setTimeout(() => controller.abort(), 60_000);
  try {
    const response = await fetch(
      `${API_BASE_URL.replace(/\/+$/, "")}${path}`,
      {
        headers: { Authorization: `Bearer ${session.accessToken}` },
        signal: controller.signal,
      },
    );
    if (!response.ok) {
      throw new LoansApiError("Không tải được chứng từ KYC.", response.status);
    }
    return await response.blob();
  } catch (error) {
    if (error instanceof LoansApiError) throw error;
    if (error instanceof DOMException && error.name === "AbortError") {
      throw new LoansApiError("Tải chứng từ quá lâu. Vui lòng thử lại.", 0);
    }
    throw new LoansApiError(
      "Không thể kết nối máy chủ. Vui lòng kiểm tra mạng và thử lại.",
      0,
    );
  } finally {
    window.clearTimeout(timer);
  }
}
