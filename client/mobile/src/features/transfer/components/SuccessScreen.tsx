import { useEffect, useRef } from "react";
import { CheckIcon } from "@/shared/icons";
import { Button } from "@/shared/ui";
import { TransactionReceipt } from "@/features/transactions";
import type { TransactionRecord } from "@/features/transactions/model";
import { formatVnd } from "@/features/transactions/utils/format";
import type { Recipient } from "@/features/transfer/model";
import { BankLogo } from "./BankLogo";

interface SuccessScreenProps {
  record: TransactionRecord;
  recipient: Recipient | null;
  onHome: () => void;
  onNewTransfer: () => void;
  onViewHistory: () => void;
}

/** Màn hình toàn màn khi chuyển tiền thành công: số tiền nổi bật, biên lai, hành động tiếp theo. */
export function SuccessScreen({
  record,
  recipient,
  onHome,
  onNewTransfer,
  onViewHistory,
}: SuccessScreenProps) {
  const heading = useRef<HTMLHeadingElement>(null);

  // Đưa focus lên tiêu đề để trình đọc màn hình thông báo kết quả ngay.
  useEffect(() => heading.current?.focus(), []);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <main className="flex-1 overflow-y-auto px-4 pb-6">
        <section className="-mx-4 flex flex-col items-center gap-2 bg-linear-to-b from-success-container/70 to-transparent px-6 pt-12 pb-6 text-center">
          <span
            aria-hidden="true"
            className="flex h-20 w-20 items-center justify-center rounded-full bg-success text-on-success shadow-lg ring-8 shadow-success/30 ring-success-container"
          >
            <CheckIcon className="h-10 w-10" />
          </span>
          <h1
            ref={heading}
            tabIndex={-1}
            className="mt-4 text-xl font-bold text-on-surface focus:outline-none"
          >
            Chuyển tiền thành công
          </h1>
          <p className="text-4xl font-bold tracking-tight text-on-surface tabular-nums">
            {formatVnd(record.amount)}
          </p>
          {recipient && (
            <div className="mt-3 flex max-w-full items-center gap-3 rounded-2xl bg-surface-container-lowest/80 px-3 py-2 text-left">
              <BankLogo bank={recipient.bank} size="sm" />
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold text-on-surface">
                  {recipient.name}
                </p>
                <p className="truncate text-xs text-on-surface-variant">
                  {recipient.accountNo} · {recipient.bank.shortName}
                </p>
              </div>
            </div>
          )}
        </section>

        <TransactionReceipt record={record} showSummary={false} />
      </main>

      <div className="space-y-3 border-t border-outline-variant/60 bg-surface-container-lowest/95 px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] backdrop-blur">
        <Button className="w-full" onClick={onHome}>
          Về trang chủ
        </Button>
        <div className="grid grid-cols-2 gap-3">
          <Button
            variant="secondary"
            className="w-full"
            onClick={onNewTransfer}
          >
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
