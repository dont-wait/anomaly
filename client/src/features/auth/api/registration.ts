import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { requestJson, ApiError } from "@/shared/lib/http";
import type { AuthUser } from "./auth";

export interface RegisterInput {
  username: string;
  cccdNumber: string;
  cccdIssuedDate: string;
  dob: string;
  email: string;
  password: string;
}
export interface VerificationMedia {
  idCardFrontUrl: string;
  idCardBackUrl: string;
  liveVideoUrl: string;
}
export async function registerAccount(
  input: RegisterInput,
  signal?: AbortSignal,
  idempotencyKey?: string,
) {
  const user = await requestJson<AuthUser>(API_ENDPOINTS.AUTH.REGISTER, {
    method: "POST",
    body: { ...input, idempotencyKey },
    signal,
  });
  if (!user?.id) throw new ApiError(0, "Phản hồi đăng ký không hợp lệ.");
  return user;
}
export async function uploadMedia(
  file: File,
  key: string,
  token: string,
  signal?: AbortSignal,
) {
  const body = new FormData();
  body.append("key", key);
  body.append("file", file);
  const result = await requestJson<{ key: string }>(
    API_ENDPOINTS.MEDIA.UPLOAD,
    {
      method: "POST",
      body,
      token,
      signal,
      timeoutMs: 120000,
    },
  );
  if (result?.key !== key)
    throw new ApiError(0, "Phản hồi tải tệp không hợp lệ.");
  return result.key;
}
export async function verifyAccount(
  id: string,
  media: VerificationMedia,
  token: string,
  signal?: AbortSignal,
) {
  const user = await requestJson<AuthUser>(API_ENDPOINTS.ACCOUNTS.VERIFY(id), {
    method: "POST",
    body: media,
    token,
    signal,
  });
  if (user?.id !== id || !user.isVerify)
    throw new ApiError(0, "Tài khoản chưa được xác thực. Vui lòng thử lại.");
  return user;
}
export function registrationError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === HTTP_STATUS.CONFLICT)
      return "Email, tên tài khoản hoặc CCCD đã được sử dụng. Vui lòng kiểm tra lại hoặc đăng nhập.";
    if (
      error.status === HTTP_STATUS.UNAUTHORIZED ||
      error.status === HTTP_STATUS.FORBIDDEN
    )
      return "Phiên đăng nhập không hợp lệ. Vui lòng đăng nhập lại để tiếp tục.";
    if (error.status === HTTP_STATUS.PAYLOAD_TOO_LARGE)
      return "Tệp quá lớn. Vui lòng chọn ảnh hoặc quay video ngắn hơn.";
    if (error.status === HTTP_STATUS.SERVICE_UNAVAILABLE)
      return "Dịch vụ xác thực chưa sẵn sàng. Vui lòng thử lại sau.";
    if (
      error.status === HTTP_STATUS.BAD_REQUEST ||
      error.status === HTTP_STATUS.UNPROCESSABLE_ENTITY
    )
      return "Dữ liệu không hợp lệ. Vui lòng kiểm tra thông tin và tệp đã chọn.";
    if (error.status >= HTTP_STATUS.INTERNAL_SERVER_ERROR)
      return "Máy chủ đang bận. Vui lòng thử lại.";
  }
  return error instanceof Error
    ? error.message
    : "Không thể hoàn tất yêu cầu. Vui lòng thử lại.";
}
