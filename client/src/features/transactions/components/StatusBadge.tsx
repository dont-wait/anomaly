import type { TransactionStatus } from "@/features/transactions/model";

const STATUS: Record<TransactionStatus, { label: string; className: string }> =
  {
    success: {
      label: "Thành công",
      className: "bg-success-container text-on-success-container",
    },
    pending: {
      label: "Đang xử lý",
      className: "bg-surface-container-highest text-primary",
    },
    failed: {
      label: "Thất bại",
      className: "bg-error-container text-on-error-container",
    },
  };

export const StatusBadge = ({ status }: { status: TransactionStatus }) => (
  <span
    className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-semibold ${STATUS[status].className}`}
  >
    {STATUS[status].label}
  </span>
);
