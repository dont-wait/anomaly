import { useState, type ReactNode } from "react";
import { CopyIcon } from "@/shared/icons";
import { StatusBadge } from "./StatusBadge";
import { TransactionIcon } from "./TransactionIcon";
import {
  KIND_LABEL,
  formatFullDateTime,
  formatSignedVnd,
  formatVnd,
  transactionTitle,
} from "@/features/transactions/utils/format";
import type { TransactionRecord } from "@/features/transactions/model";

const Row = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex items-start justify-between gap-4 py-3">
    <dt className="shrink-0 text-sm text-on-surface-variant">{label}</dt>
    <dd className="min-w-0 text-right text-sm font-medium break-words text-on-surface">
      {children}
    </dd>
  </div>
);

interface TransactionReceiptProps {
  record: TransactionRecord;
  /** Ẩn phần đầu (icon + số tiền) khi màn hình cha đã tự hiển thị */
  showSummary?: boolean;
}

/** Biên lai giao dịch — dùng cho trang chi tiết và màn kết quả chuyển tiền. */
export const TransactionReceipt = ({
  record,
  showSummary = true,
}: TransactionReceiptProps) => {
  const [copyMessage, setCopyMessage] = useState("");
  const counterpartyLabel =
    record.direction === "in" ? "Người gửi" : "Người nhận";
  const amountTone =
    record.status === "failed"
      ? "text-on-surface-variant line-through"
      : record.direction === "in"
        ? "text-success"
        : "text-on-surface";

  const copyReference = async () => {
    try {
      await navigator.clipboard.writeText(record.reference);
      setCopyMessage("Đã sao chép mã giao dịch");
    } catch {
      setCopyMessage("Không thể sao chép mã giao dịch. Vui lòng thử lại.");
    }
  };

  return (
    <section className="rounded-3xl bg-surface-container-lowest p-5 shadow-sm shadow-secondary/10">
      {showSummary && (
        <div className="flex flex-col items-center gap-2 border-b border-dashed border-outline-variant pb-5 text-center">
          <TransactionIcon record={record} size="lg" />
          <p className="text-sm text-on-surface-variant">
            {transactionTitle(record)}
          </p>
          <p
            className={`text-3xl font-bold tracking-tight tabular-nums ${amountTone}`}
          >
            {formatSignedVnd(record)}
          </p>
          <StatusBadge status={record.status} />
        </div>
      )}

      <dl className="divide-y divide-outline-variant/50">
        <Row label="Thời gian">{formatFullDateTime(record.createdAt)}</Row>
        <Row label="Mã giao dịch">
          <button
            type="button"
            onClick={copyReference}
            aria-label={`Sao chép mã giao dịch ${record.reference}`}
            className="-my-1 inline-flex min-h-8 items-center gap-1.5 rounded-full px-2 font-mono text-secondary-strong hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
          >
            {record.reference}
            <CopyIcon className="h-4 w-4" />
          </button>
        </Row>
        <Row label="Loại giao dịch">{KIND_LABEL[record.kind]}</Row>
        <Row label={counterpartyLabel}>
          <span className="block">{record.counterparty.name}</span>
          {record.counterparty.accountNo && (
            <span className="block text-xs font-normal text-on-surface-variant">
              {record.counterparty.accountNo}
              {record.counterparty.bank && ` · ${record.counterparty.bank}`}
            </span>
          )}
        </Row>
        <Row label="Nội dung">{record.note || "—"}</Row>
        <Row label="Phí giao dịch">
          {record.fee === 0 ? "Miễn phí" : formatVnd(record.fee)}
        </Row>
        {record.balanceAfter !== undefined && (
          <Row label="Số dư sau GD">{formatVnd(record.balanceAfter)}</Row>
        )}
      </dl>
      <p
        role="status"
        className="mt-1 min-h-4 text-center text-xs text-on-surface-variant"
      >
        {copyMessage}
      </p>
    </section>
  );
};
