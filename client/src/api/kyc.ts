import { ApiError, requestJson } from "./http";
export const KYC_BASE_URL =
  import.meta.env.VITE_KYC_ENDPOINT || "http://localhost:8090";
export const LIVENESS_CHALLENGE = "TURN_HEAD_LEFT_RIGHT_BLINK";
export type KycDecision =
  "VERIFIED" | "RETRY_ALLOWED" | "FAILED_FINAL" | "SYSTEM_ERROR";
export interface KycResult {
  success: boolean;
  decision: KycDecision;
  reason_code: string | null;
  reason_message: string | null;
}
export async function verifyFace(
  front: File,
  video: File,
  signal?: AbortSignal,
): Promise<KycResult> {
  const body = new FormData();
  body.append("cccd_front_image", front);
  body.append("live_video", video);
  body.append("challenge_type", LIVENESS_CHALLENGE);
  const result = await requestJson<KycResult>("/v1/kyc/verify-face", {
    method: "POST",
    body,
    baseUrl: KYC_BASE_URL,
    signal,
    timeoutMs: 120000,
  });
  if (
    !result ||
    typeof result.success !== "boolean" ||
    !["VERIFIED", "RETRY_ALLOWED", "FAILED_FINAL", "SYSTEM_ERROR"].includes(
      result.decision,
    ) ||
    (result.decision === "VERIFIED") !== result.success
  ) {
    throw new ApiError(0, "Phản hồi xác thực không hợp lệ. Vui lòng thử lại.");
  }
  return result;
}
