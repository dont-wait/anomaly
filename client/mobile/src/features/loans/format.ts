import { ApiError } from "@/shared/lib/http";
export const vnd = (amount: number) =>
  `${new Intl.NumberFormat("vi-VN").format(amount)} đ`;
export const percentBps = (bps: number) =>
  `${new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 2 }).format(bps / 100)}%/năm`;
export const explainLoanError = (error: unknown) => {
  if (error instanceof ApiError) {
    if (error.hasCode("INSTALLMENT_NOT_DUE"))
      return "Chưa đến ngày thanh toán. Ngân hàng chỉ hỗ trợ trả đúng ngày đến hạn.";
    if (error.hasCode("INVALID_INSTALLMENT_ORDER"))
      return "Phải thanh toán các kỳ theo đúng thứ tự.";
    if (error.hasCode("QUOTE_EXPIRED"))
      return "Báo giá đã hết hạn. Vui lòng lấy báo giá mới.";
    if (error.hasCode("CONTRACT_PROJECTION_PENDING"))
      return "Hợp đồng đang được đồng bộ, hãy tải lại sau.";
    if (error.hasCode("CONTRACT_LIST_CHANGED"))
      return "Danh sách hợp đồng đã thay đổi, vui lòng tải lại từ trang đầu.";
    if (error.hasCode("INSUFFICIENT_FUNDS"))
      return "Tài khoản không đủ số dư để trả kỳ vay.";
    if (error.hasCode("INSTALLMENT_ALREADY_PAID"))
      return "Kỳ vay đã được thanh toán, hãy tải lại lịch trả nợ.";
    return error.message;
  }
  return error instanceof Error
    ? error.message
    : "Có lỗi xảy ra. Vui lòng thử lại.";
};
