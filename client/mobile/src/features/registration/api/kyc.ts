import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { ApiError, requestApi } from "@/shared/lib/http";
import type { AuthUser } from "@/features/auth/api";
export const LIVENESS_CHALLENGE = "TURN_HEAD_LEFT_RIGHT_BLINK";
export type KycDecision =
  "VERIFIED" | "RETRY_ALLOWED" | "FAILED_FINAL" | "SYSTEM_ERROR";
export interface KycResult {
  decision: KycDecision;
  reasonCode?: string;
  reasonMessage?: string;
  user?: AuthUser;
}
const decisions: KycDecision[] = [
  "VERIFIED",
  "RETRY_ALLOWED",
  "FAILED_FINAL",
  "SYSTEM_ERROR",
];

function validateResult(value: unknown): KycResult {
  if (!value || typeof value !== "object")
    throw new ApiError(0, "Phản hồi xác thực không hợp lệ. Vui lòng thử lại.");
  const result = value as KycResult;
  if (
    !decisions.includes(result.decision) ||
    (result.reasonCode !== undefined && typeof result.reasonCode !== "string") ||
    (result.reasonMessage !== undefined &&
      typeof result.reasonMessage !== "string") ||
    (result.decision === "VERIFIED"
      ? !result.user?.id || result.user.isVerify !== true
      : result.user !== undefined)
  ) {
    throw new ApiError(0, "Phản hồi xác thực không hợp lệ. Vui lòng thử lại.");
  }
  return result;
}

function wrappedErrorData(error: ApiError): unknown {
  if (!error.body || typeof error.body !== "object") return undefined;
  return (error.body as { data?: unknown }).data;
}

export async function completeKyc(
  front: File,
  back: File,
  video: File,
  token: string,
  signal?: AbortSignal,
): Promise<KycResult> {
  const body = new FormData();
  body.append("idCardFront", front);
  body.append("idCardBack", back);
  body.append("liveVideo", video);
  body.append("challengeType", LIVENESS_CHALLENGE);
  try {
    return validateResult(
      await requestApi<KycResult>(API_ENDPOINTS.KYC.COMPLETE, {
        method: "POST",
        body,
        token,
        signal,
        timeoutMs: 120000,
      }),
    );
  } catch (error) {
    if (error instanceof ApiError && error.status === 502) {
      const data = wrappedErrorData(error);
      if (
        data &&
        typeof data === "object" &&
        (data as { decision?: unknown }).decision === "SYSTEM_ERROR"
      ) {
        return validateResult(data);
      }
    }
    throw error;
  }
}
