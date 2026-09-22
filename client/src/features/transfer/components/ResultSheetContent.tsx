import { CheckIcon, CloseIcon } from "@/shared/icons";
import { Button } from "@/shared/ui";
import { TransactionReceipt } from "@/features/transactions";
import { formatVnd } from "@/features/transactions/utils/format";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";

type Props = Pick<
  TransferFlowState,
  "result" | "retry" | "reset" | "closeSheet"
> & {
  onHome: () => void;
  onViewHistory: () => void;
};

export function ResultSheetContent({
  result,
  retry,
  reset,
  closeSheet,
  onHome,
  onViewHistory,
}: Props) {
  if (!result) return null;

  if (!result.ok) {
    return (
      <div className="space-y-6">
        <div className="flex flex-col items-center gap-3 pt-2 text-center">
          <span
            aria-hidden="true"
            className="flex h-16 w-16 items-center justify-center rounded-full bg-error-container text-error"
          >
            <CloseIcon className="h-8 w-8" />
          </span>
          <p className="text-lg font-bold text-on-surface">
            Chuyển tiền thất bại
          </p>
          <p role="alert" className="text-sm text-on-surface-variant">
            {result.message}
          </p>
        </div>
        <div className="space-y-3">
          <Button className="w-full" onClick={retry}>
            Thử lại
          </Button>
          <Button variant="secondary" className="w-full" onClick={closeSheet}>
            Sửa thông tin
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col items-center gap-2 text-center">
        <span
          aria-hidden="true"
          className="flex h-16 w-16 items-center justify-center rounded-full bg-success-container text-success"
        >
          <CheckIcon className="h-8 w-8" />
        </span>
        <p role="status" className="text-lg font-bold text-on-surface">
          Chuyển tiền thành công
        </p>
        <p className="text-3xl font-bold tracking-tight text-on-surface tabular-nums">
          {formatVnd(result.record.amount)}
        </p>
        <p className="text-sm text-on-surface-variant">
          đến{" "}
          <span className="font-semibold text-on-surface">
            {result.record.counterparty.name}
          </span>
        </p>
      </div>

      <TransactionReceipt record={result.record} showSummary={false} />

      <div className="space-y-3">
        <Button className="w-full" onClick={onHome}>
          Về trang chủ
        </Button>
        <div className="grid grid-cols-2 gap-3">
          <Button variant="secondary" className="w-full" onClick={reset}>
            Giao dịch mới
          </Button>
          <Button
            variant="secondary"
            className="w-full"
            onClick={onViewHistory}
          >
            Xem lịch sử
          </Button>
        </div>
      </div>
    </div>
  );
}
