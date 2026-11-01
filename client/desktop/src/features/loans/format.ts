import { LoansApiError } from "./api/loansApi";

export const vnd = (amount: number) =>
  `${new Intl.NumberFormat("vi-VN").format(amount)} đ`;

export const statusLabels: Record<string, string> = {
  draft: "Chưa gửi",
  submitted: "Chờ duyệt",
  under_review: "Đang thẩm định",
  approved: "Đã duyệt",
  rejected: "Từ chối",
  disbursed: "Đã giải ngân",
};

const ERROR_BY_STATUS: Record<number, string> = {
  400: "Thông tin chưa hợp lệ. Vui lòng kiểm tra lại.",
  409: "Hồ sơ vừa thay đổi hoặc chưa đủ điều kiện. Vui lòng tải lại rồi thử lại.",
  422: "Hồ sơ chưa đủ điều kiện cho thao tác này.",
  429: "Thao tác quá nhanh. Vui lòng đợi rồi thử lại.",
};

export const assessmentLabels: Record<string, string> = {
  auto_approve_eligible: "Đủ điều kiện duyệt",
  manual_review: "Cần quyết định thủ công",
  rejected: "Không đạt",
};

export const offerLabels: Record<string, string> = {
  issued: "Đã gửi, chờ khách phản hồi",
  accepted: "Khách đã chấp nhận",
  rejected: "Khách đã từ chối",
  confirmed: "Khách đã xác nhận hợp đồng",
};

export const documentStatusLabels: Record<string, string> = {
  pending: "Chờ xác minh",
  verified: "Đã xác minh",
  rejected: "Không đạt",
};

export const documentLabels: Record<string, string> = {
  IDENTITY_FRONT: "CCCD mặt trước",
  IDENTITY_BACK: "CCCD mặt sau",
  VNEID_PERSONAL: "VNeID - thông tin cá nhân",
  VNEID_RESIDENCE: "VNeID - cư trú",
  VSSID_PARTICIPATION: "VssID - tham gia BHXH",
  VSSID_DETAIL: "VssID - chi tiết BHXH",
  SALARY_STATEMENT_3_MONTHS: "Sao kê lương 3 tháng",
};

export function explainLoanError(error: unknown): string {
  if (error instanceof LoansApiError) {
    if (error.status === 401)
      return "Phiên đăng nhập admin đã hết hạn. Vui lòng đăng nhập lại.";
    if (error.status === 403)
      return "Tài khoản hiện tại không có quyền thực hiện thao tác này.";
    if (error.status === 404) return "Không tìm thấy hồ sơ vay.";
    if (error.hasCode("CONTRACT_PROJECTION_PENDING")) {
      return "Hợp đồng đang được đồng bộ, hãy tải lại sau.";
    }
    if (error.status === 0) return error.message;
    // Thông báo từ máy chủ có thể là tiếng Anh hoặc thuật ngữ nội bộ.
    return (
      ERROR_BY_STATUS[error.status] ??
      "Hệ thống đang gặp sự cố. Vui lòng thử lại sau."
    );
  }
  return error instanceof Error
    ? error.message
    : "Có lỗi xảy ra. Vui lòng thử lại.";
}
