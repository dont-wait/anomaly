import { ApiError, requestJson } from "./http";
export const KYC_BASE_URL =
  import.meta.env.VITE_KYC_ENDPOINT || "http://localhost:8090";
export function validateKycEndpoint(
  endpoint: string,
  development = import.meta.env.DEV,
): string {
  const url = new URL(endpoint);
  const loopback =
    url.hostname === "localhost" ||
    url.hostname === "[::1]" ||
    /^127\.\d+\.\d+\.\d+$/.test(url.hostname);
  if (
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    (url.protocol !== "https:" &&
      !(development && loopback && url.protocol === "http:"))
  ) {
    throw new Error(
      "KYC yêu cầu HTTPS; HTTP chỉ được dùng trên loopback khi phát triển.",
    );
  }
  return url.toString().replace(/\/+$/, "");
}
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
  const baseUrl = validateKycEndpoint(KYC_BASE_URL);
  const body = new FormData();
  body.append("cccd_front_image", front);
  body.append("live_video", video);
  body.append("challenge_type", LIVENESS_CHALLENGE);
  const result = await requestJson<KycResult>("/v1/kyc/verify-face", {
    method: "POST",
    body,
    baseUrl,
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
