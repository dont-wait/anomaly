import { ApiError } from "@/shared/lib/http";

export const AVATAR_TYPES = ["image/jpeg", "image/png", "image/webp"];
export const AVATAR_MAX_BYTES = 5 * 1024 * 1024;
export const DOCUMENT_TYPES = ["application/pdf", "image/jpeg", "image/png"];
export const DOCUMENT_MAX_BYTES = 10 * 1024 * 1024;

export const PHONE_PATTERN = /^\+?[0-9]{8,16}$/;
export const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** Chỉ giữ 4 ký tự cuối, kể cả khi backend trả về giá trị đầy đủ. */
export function maskTail(value?: string) {
  if (!value) return "";
  return `•••• ${value.slice(-4)}`;
}

export function validateUpload(
  file: File,
  allowedTypes: string[],
  maxBytes: number,
) {
  if (file.size <= 0) return "Tệp đang trống.";
  if (file.size > maxBytes) {
    return `Tệp vượt quá ${Math.round(maxBytes / (1024 * 1024))} MiB.`;
  }
  if (!allowedTypes.includes(file.type)) {
    return "Định dạng tệp không được hỗ trợ.";
  }
  return "";
}

export function documentLabel(type: string) {
  switch (type) {
    case "IDENTITY_FRONT":
      return "CCCD mặt trước";
    case "IDENTITY_BACK":
      return "CCCD mặt sau";
    case "INCOME_PROOF":
      return "Chứng từ thu nhập";
    case "ADDRESS_PROOF":
      return "Chứng từ địa chỉ";
    default:
      return type;
  }
}

export function formatDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toLocaleDateString("vi-VN");
}

/** Lỗi backend có thể bằng tiếng Anh, nên hiển thị theo mã trạng thái của spec. */
export function toProfileError(
  cause: unknown,
  badRequest = "Thông tin chưa hợp lệ. Vui lòng kiểm tra lại.",
) {
  if (!(cause instanceof ApiError)) {
    return cause instanceof Error ? cause.message : "Không thể cập nhật hồ sơ.";
  }
  switch (cause.status) {
    case 0:
      return cause.message;
    case 400:
      return badRequest;
    case 401:
      return "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";
    case 403:
      return "Thao tác này không được phép ở trạng thái hiện tại.";
    case 404:
      return "Không tìm thấy dữ liệu yêu cầu.";
    case 409:
      return "Hồ sơ vừa thay đổi hoặc dữ liệu đã được sử dụng. Vui lòng kiểm tra rồi thử lại.";
    case 413:
      return "Tệp quá lớn.";
    case 415:
      return "Định dạng tệp không được hỗ trợ.";
    case 429:
      return "Bạn đã thử quá nhiều lần. Vui lòng đợi rồi thử lại.";
    default:
      return "Hệ thống đang gặp sự cố. Vui lòng thử lại sau.";
  }
}
