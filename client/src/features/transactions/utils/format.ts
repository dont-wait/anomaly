import type {
  TransactionKind,
  TransactionRecord,
} from "@/features/transactions/model";

const numberFormat = new Intl.NumberFormat("vi-VN");

export const formatVnd = (amount: number) => `${numberFormat.format(amount)}₫`;

export const formatSignedVnd = (record: TransactionRecord) =>
  `${record.direction === "in" ? "+" : "-"}${formatVnd(record.amount)}`;

export const maskAccountNo = (accountNo: string) =>
  `**** ${accountNo.slice(-4)}`;

export const formatTime = (date: Date) =>
  new Intl.DateTimeFormat("vi-VN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);

export const formatFullDateTime = (date: Date) =>
  `${formatTime(date)}, ${new Intl.DateTimeFormat("vi-VN", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(date)}`;

const dayKey = (date: Date) =>
  `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;

export function formatDayLabel(date: Date, now = new Date()) {
  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  if (dayKey(date) === dayKey(now)) return "Hôm nay";
  if (dayKey(date) === dayKey(yesterday)) return "Hôm qua";
  const label = new Intl.DateTimeFormat("vi-VN", {
    weekday: "long",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(date);
  return label.charAt(0).toUpperCase() + label.slice(1);
}

export interface TransactionGroup {
  key: string;
  label: string;
  items: TransactionRecord[];
}

/** Nhóm giao dịch (đã sắp xếp mới nhất trước) theo ngày. */
export function groupByDay(
  records: TransactionRecord[],
  now = new Date(),
): TransactionGroup[] {
  const groups = new Map<string, TransactionGroup>();
  for (const record of records) {
    const key = dayKey(record.createdAt);
    const group = groups.get(key);
    if (group) group.items.push(record);
    else
      groups.set(key, {
        key,
        label: formatDayLabel(record.createdAt, now),
        items: [record],
      });
  }
  return [...groups.values()];
}

export const KIND_LABEL: Record<TransactionKind, string> = {
  transfer: "Chuyển khoản",
  payment: "Thanh toán",
  bill: "Hóa đơn",
  topup: "Nạp tiền ĐT",
  savings: "Tiết kiệm",
  salary: "Lương",
};

export function transactionTitle(record: TransactionRecord) {
  if (record.kind !== "transfer") return record.counterparty.name;
  return record.direction === "in"
    ? `Nhận từ ${record.counterparty.name}`
    : `Chuyển đến ${record.counterparty.name}`;
}
