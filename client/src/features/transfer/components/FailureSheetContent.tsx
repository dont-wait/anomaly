import { CloseIcon } from "@/shared/icons";
import { Button } from "@/shared/ui";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";

type Props = Pick<TransferFlowState, "result" | "retry" | "closeSheet">;

/** Nội dung sheet khi giao dịch thất bại — cho thử lại hoặc quay về sửa form. */
export function FailureSheetContent({ result, retry, closeSheet }: Props) {
  if (!result || result.ok) return null;

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
