import type { Ref } from "react";
import { CheckIcon, CloseIcon } from "@/shared/icons";
import { TransactionReceipt } from "@/features/transactions";
import { formatVnd } from "@/features/transactions/utils/format";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";
import { Button } from "@/shared/ui";

type Props = Pick<TransferFlowState, "result" | "retry" | "reset"> & {
  headingRef: Ref<HTMLHeadingElement>;
  onHome: () => void;
  onViewHistory: () => void;
};

export function ResultStep({
  result,
  retry,
  reset,
  headingRef,
  onHome,
  onViewHistory,
}: Props) {
  if (!result) return null;

  if (!result.ok) {
    return (
      <div className="space-y-6">
        <div className="flex flex-col items-center gap-3 pt-4 text-center">
          <span
            aria-hidden="true"
            className="flex h-20 w-20 items-center justify-center rounded-full bg-error-container text-error"
          >
            <CloseIcon className="h-10 w-10" />
          </span>
          <h2
            ref={headingRef}
            tabIndex={-1}
            className="text-xl font-bold text-on-surface focus:outline-none"
          >
            Chuyển tiền thất bại
          </h2>
          <p className="text-sm text-on-surface-variant">{result.message}</p>
        </div>
        <div className="space-y-3">
          <Button type="button" onClick={retry} className="w-full">
            Thử lại
          </Button>
          <Button
            variant="secondary"
            type="button"
            onClick={onHome}
            className="w-full"
          >
            Về trang chủ
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-col items-center gap-3 pt-2 text-center">
        <span
          aria-hidden="true"
          className="flex h-20 w-20 items-center justify-center rounded-full bg-success-container text-success"
        >
          <CheckIcon className="h-10 w-10" />
        </span>
        <h2
          ref={headingRef}
          tabIndex={-1}
          className="text-xl font-bold text-on-surface focus:outline-none"
        >
          Chuyển tiền thành công
        </h2>
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
        <Button type="button" onClick={onHome} className="w-full">
          Về trang chủ
        </Button>
        <div className="grid grid-cols-2 gap-3">
          <Button
            variant="secondary"
            type="button"
            onClick={reset}
            className="w-full"
          >
            Giao dịch mới
          </Button>
          <Button
            variant="secondary"
            type="button"
            onClick={onViewHistory}
            className="w-full"
          >
            Xem lịch sử
          </Button>
        </div>
      </div>
    </div>
  );
}
