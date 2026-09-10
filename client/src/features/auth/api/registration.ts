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
  const user = await requestJson<AuthUser>("/api/auth/register", {
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
  const result = await requestJson<{ key: string }>("/api/media/upload", {
    method: "POST",
    body,
    token,
    signal,
    timeoutMs: 120000,
  });
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
  const user = await requestJson<AuthUser>(
    `/api/accounts/${encodeURIComponent(id)}/verify`,
    { method: "POST", body: media, token, signal },
  );
  if (user?.id !== id || !user.isVerify)
    throw new ApiError(0, "Tài khoản chưa được xác thực. Vui lòng thử lại.");
  return user;
}
export function registrationError(error: unknown) {
  if (error instanceof ApiError) {
    if (error.status === 409)
      return "Email, tên tài khoản hoặc CCCD đã được sử dụng. Vui lòng kiểm tra lại hoặc đăng nhập.";
    if (error.status === 401 || error.status === 403)
      return "Phiên đăng nhập không hợp lệ. Vui lòng đăng nhập lại để tiếp tục.";
    if (error.status === 413)
      return "Tệp quá lớn. Vui lòng chọn ảnh hoặc quay video ngắn hơn.";
    if (error.status === 503)
      return "Dịch vụ xác thực chưa sẵn sàng. Vui lòng thử lại sau.";
    if (error.status === 400 || error.status === 422)
      return "Dữ liệu không hợp lệ. Vui lòng kiểm tra thông tin và tệp đã chọn.";
    if (error.status >= 500) return "Máy chủ đang bận. Vui lòng thử lại.";
  }
  return error instanceof Error
    ? error.message
    : "Không thể hoàn tất yêu cầu. Vui lòng thử lại.";
}
