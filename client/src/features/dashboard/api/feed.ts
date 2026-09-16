import { requestJson } from "@/shared/lib/http";
import type { Transaction, TransactionCategory } from "@/features/dashboard/model/types";

interface FeedEntryResponse {
  id: string;
  transactionId: string;
  direction: "in" | "out";
  type: string;
  counterpartyName: string;
  counterpartyNo: string;
  amount: number;
  balanceAfter: number;
  occurredAt: string;
}

function toTransaction(entry: FeedEntryResponse): Transaction {
  const category: TransactionCategory =
    entry.direction === "in" ? "transfer-in" : "transfer-out";
  return {
    id: entry.id,
    amount: entry.amount,
    type: entry.direction === "in" ? "credit" : "debit",
    description:
      entry.direction === "in"
        ? `Nhận từ ${entry.counterpartyName}`
        : `Chuyển đến ${entry.counterpartyName}`,
    subtitle: entry.counterpartyNo,
    category,
    date: new Date(entry.occurredAt),
  };
}

export async function getAccountFeed(
  accountId: string,
  token: string,
  options: { signal?: AbortSignal; limit?: number } = {},
): Promise<Transaction[]> {
  const list = await requestJson<FeedEntryResponse[]>(
    `/api/transactions${options.limit ? `?limit=${options.limit}` : ""}`,
    { token, signal: options.signal },
  );
  return (list ?? []).map(toTransaction);
}