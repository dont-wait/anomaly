import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { ApiError, requestJson } from "@/shared/lib/http";

export interface RequestOtpResponse {
  message: string;
}
export interface VerifyOtpResponse {
  verified: boolean;
}

export async function requestOtp(
  email: string,
  signal?: AbortSignal,
): Promise<string> {
  const result = await requestJson<RequestOtpResponse>(
    API_ENDPOINTS.AUTH.OTP_REQUEST,
    {
      method: "POST",
      body: { email: email.trim() },
      signal,
    },
  );
  if (typeof result?.message !== "string")
    throw new ApiError(0, "Phản hồi gửi mã OTP không hợp lệ.");
  return result.message;
}
export async function verifyOtp(
  email: string,
  code: string,
  signal?: AbortSignal,
): Promise<boolean> {
  const result = await requestJson<VerifyOtpResponse>(
    API_ENDPOINTS.AUTH.OTP_VERIFY,
    {
      method: "POST",
      body: { email: email.trim(), code: code.trim() },
      signal,
    },
  );
  if (result?.verified !== true)
    throw new ApiError(0, "Phản hồi xác thực OTP không hợp lệ.");
  return true;
}
export function otpError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === HTTP_STATUS.GONE)
      return "Mã OTP đã hết hạn hoặc bạn đã nhập sai quá số lần cho phép. Vui lòng gửi lại mã mới.";
    if (error.status === HTTP_STATUS.BAD_REQUEST)
      return "Mã OTP không đúng hoặc dữ liệu không hợp lệ. Vui lòng kiểm tra lại.";
    if (error.status === HTTP_STATUS.PAYLOAD_TOO_LARGE)
      return "Dữ liệu gửi đi quá lớn. Vui lòng thử lại.";
    if (error.status >= HTTP_STATUS.INTERNAL_SERVER_ERROR)
      return "Máy chủ đang bận. Vui lòng thử lại.";
  }
  return error instanceof Error
    ? error.message
    : "Không thể hoàn tất yêu cầu. Vui lòng thử lại.";
}
