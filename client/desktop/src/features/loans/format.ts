import { LoansApiError } from "./api/loansApi";

export const vnd = (amount: number) =>
  `${new Intl.NumberFormat("vi-VN").format(amount)} đ`;

export const statusLabels: Record<string, string> = {
  draft: "Nháp",
  submitted: "Đã nộp",
  under_review: "Đang thẩm định",
  approved: "Đã duyệt",
  rejected: "Từ chối",
  disbursed: "Đã giải ngân",
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
    return error.message;
  }
  return error instanceof Error
    ? error.message
    : "Có lỗi xảy ra. Vui lòng thử lại.";
}
