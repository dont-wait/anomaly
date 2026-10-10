import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import { HTTP_STATUS } from "@/shared/constants/httpStatus";
import { requestApi, ApiError } from "@/shared/lib/http";
import type { AuthUser } from "@/features/auth/api";

export interface RegisterInput {
  username: string;
  cccdNumber: string;
  cccdIssuedDate: string;
  dob: string;
  email: string;
  password: string;
}
export interface RegisterResult {
  user: AuthUser;
  kycToken?: string;
  kycExpiresAt?: string;
}
export async function registerAccount(
  input: RegisterInput,
  signal?: AbortSignal,
  idempotencyKey?: string,
) {
  const result = await requestApi<RegisterResult>(API_ENDPOINTS.AUTH.REGISTER, {
    method: "POST",
    body: { ...input, idempotencyKey },
    signal,
  });
  if (
    !result?.user?.id ||
    typeof result.user.isVerify !== "boolean" ||
    (!result.user.isVerify &&
      (!result.kycToken ||
        !result.kycExpiresAt ||
        !Number.isFinite(Date.parse(result.kycExpiresAt))))
  )
    throw new ApiError(0, "Phản hồi đăng ký không hợp lệ.");
  return result;
}
export function registrationError(error: unknown) {
  if (error instanceof ApiError) {
    if (
      error.hasCode("USER_ALREADY_EXISTS") ||
      error.hasCode("IDEMPOTENCY_CONFLICT")
    )
      return "Email, tên tài khoản hoặc CCCD đã được sử dụng. Vui lòng kiểm tra lại hoặc đăng nhập.";
    if (error.hasCode("INVALID_EMAIL"))
      return "Địa chỉ email không hợp lệ. Vui lòng kiểm tra lại.";
    if (error.hasCode("INVALID_CCCD"))
      return "Số CCCD phải gồm đúng 12 chữ số.";
    if (error.hasCode("WEAK_PASSWORD"))
      return "Mật khẩu phải có ít nhất 8 ký tự.";
    if (error.hasCode("INVALID_USERNAME"))
      return "Vui lòng nhập tên tài khoản.";
    if (error.hasCode("INVALID_DATE"))
      return "Ngày sinh hoặc ngày cấp CCCD không hợp lệ.";
    if (error.hasCode("INVALID_VERIFY_PAYLOAD"))
      return "Vui lòng hoàn tất đầy đủ thông tin xác thực.";
    if (error.hasCode("INVALID_TOKEN") || error.hasCode("MISSING_AUTH_HEADER"))
      return "Phiên xác thực danh tính không hợp lệ. Vui lòng đăng nhập để tiếp tục.";
    if (error.hasCode("FILE_TOO_LARGE"))
      return "Tệp quá lớn. Vui lòng chọn ảnh hoặc quay video ngắn hơn.";
    if (error.status === HTTP_STATUS.CONFLICT)
      return "Email, tên tài khoản hoặc CCCD đã được sử dụng. Vui lòng kiểm tra lại hoặc đăng nhập.";
    if (
      error.status === HTTP_STATUS.UNAUTHORIZED ||
      error.status === HTTP_STATUS.FORBIDDEN
    )
      return "Phiên xác thực danh tính không hợp lệ. Vui lòng đăng nhập để tiếp tục.";
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
