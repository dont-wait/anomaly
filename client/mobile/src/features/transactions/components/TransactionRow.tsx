import { StatusBadge } from "./StatusBadge";
import { TransactionIcon } from "./TransactionIcon";
import {
  KIND_LABEL,
  formatSignedVnd,
  formatTime,
  transactionTitle,
} from "@/features/transactions/utils/format";
import type { TransactionRecord } from "@/features/transactions/model";

interface TransactionRowProps {
  record: TransactionRecord;
  onSelect: (record: TransactionRecord) => void;
}

export const TransactionRow = ({ record, onSelect }: TransactionRowProps) => {
  const isFailed = record.status === "failed";
  const amountTone = isFailed
    ? "text-on-surface-variant line-through"
    : record.direction === "in"
      ? "text-success"
      : "text-on-surface";

  return (
    <button
      type="button"
      onClick={() => onSelect(record)}
      className="flex w-full items-center gap-3 rounded-2xl px-3 py-3 text-left transition-colors hover:bg-surface-container-low focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
    >
      <TransactionIcon record={record} />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium text-on-surface">
          {transactionTitle(record)}
        </span>
        <span className="block truncate text-xs text-on-surface-variant">
          {formatTime(record.createdAt)} · {KIND_LABEL[record.kind]}
        </span>
      </span>
      <span className="flex shrink-0 flex-col items-end gap-1">
        <span className={`text-sm font-semibold tabular-nums ${amountTone}`}>
          {formatSignedVnd(record)}
        </span>
        {record.status !== "success" && <StatusBadge status={record.status} />}
      </span>
    </button>
  );
};
