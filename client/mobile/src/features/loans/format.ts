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
    // Thông báo từ máy chủ có thể là tiếng Anh hoặc thuật ngữ nội bộ. Mã 0 là
    // lỗi kết nối do chính app tạo bằng tiếng Việt.
    if (error.status === 0) return error.message;
    return (
      ERROR_BY_STATUS[error.status] ??
      "Hệ thống đang gặp sự cố. Vui lòng thử lại sau."
    );
  }
  // Lỗi không phải ApiError do chính app tạo bằng tiếng Việt.
  return error instanceof Error
    ? error.message
    : "Có lỗi xảy ra. Vui lòng thử lại.";
};

export const applicationStatusMeta: Record<
  string,
  { label: string; className: string }
> = {
  draft: { label: "Chưa gửi", className: "bg-gray-100 text-gray-700" },
  submitted: { label: "Chờ duyệt", className: "bg-blue-50 text-blue-700" },
  under_review: {
    label: "Đang thẩm định",
    className: "bg-amber-50 text-amber-800",
  },
  approved: { label: "Đã duyệt", className: "bg-emerald-50 text-emerald-700" },
  rejected: { label: "Từ chối", className: "bg-red-50 text-red-700" },
  disbursed: {
    label: "Đã giải ngân",
    className: "bg-violet-50 text-violet-700",
  },
};

export const dtiStatusMeta: Record<
  string,
  { label: string; className: string }
> = {
  pass: { label: "Phù hợp", className: "bg-emerald-50 text-emerald-700" },
  manual_review: {
    label: "Cần xem xét thêm",
    className: "bg-amber-50 text-amber-800",
  },
  reject: { label: "Vượt ngưỡng", className: "bg-red-50 text-red-700" },
};

/** "50.000.000" đọc dễ nhầm số 0, nên kèm cách đọc rút gọn: "50 triệu đồng". */
export const moneyInWords = (value: number) => {
  if (value <= 0) return "";
  const units: Array<[number, string]> = [
    [1_000_000_000, "tỷ"],
    [1_000_000, "triệu"],
    [1_000, "nghìn"],
  ];
  for (const [unit, name] of units) {
    if (value >= unit) {
      const amount = new Intl.NumberFormat("vi-VN", {
        maximumFractionDigits: 3,
      }).format(value / unit);
      return `${amount} ${name} đồng`;
    }
  }
  return `${value} đồng`;
};

const STATUS_LABELS: Record<string, string> = {
  pending: "Đang chờ xác minh",
  verified: "Đã xác minh",
  rejected: "Không đạt",
  issued: "Chờ bạn phản hồi",
  accepted: "Đã chấp nhận",
  confirmed: "Đã xác nhận",
  active: "Đang vay",
  completed: "Đã tất toán",
  upcoming: "Chưa đến hạn",
  paid: "Đã thanh toán",
  success: "Thành công",
  succeeded: "Thành công",
  failed: "Không thành công",
};

/** Không bao giờ hiển thị giá trị thô từ hệ thống cho khách hàng. */
export const statusLabel = (status: string | null | undefined) =>
  STATUS_LABELS[status ?? ""] ?? "Đang cập nhật";

export const applicationStatusLabel = (status: string) =>
  applicationStatusMeta[status]?.label ?? "Đang cập nhật";

const ERROR_BY_STATUS: Record<number, string> = {
  400: "Thông tin chưa hợp lệ. Vui lòng kiểm tra lại.",
  401: "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.",
  403: "Bạn không thể thực hiện thao tác này ở trạng thái hiện tại.",
  404: "Không tìm thấy dữ liệu yêu cầu.",
  409: "Dữ liệu vừa thay đổi. Vui lòng tải lại rồi thử lại.",
  413: "Tệp quá lớn.",
  415: "Định dạng tệp không được hỗ trợ.",
  429: "Bạn thao tác quá nhanh. Vui lòng đợi rồi thử lại.",
};
