import { requestJson } from "@/shared/lib/http";
import type { TransactionDirection, TransactionRecord } from "@/features/transactions/model";
import { toRecord } from "@/features/transfer/api/transfer";

interface TransactionDto extends Omit<TransactionRecord, "createdAt" | "counterparty"> {
  createdAt: string;
  counterparty: { name: string; accountNo?: string; bank?: string; bankCode?: string };
}

export interface TransactionPage {
  items: TransactionRecord[];
  nextCursor: string | null;
}

export interface TransactionSummary {
  month: string;
  totalIn: number;
  totalOut: number;
}

export async function listTransactions(token: string, options: {
  direction?: "all" | TransactionDirection;
  query?: string;
  cursor?: string;
  limit?: number;
} = {}): Promise<TransactionPage> {
  const params = new URLSearchParams({ direction: options.direction ?? "all", limit: String(options.limit ?? 20) });
  if (options.query?.trim()) params.set("q", options.query.trim());
  if (options.cursor) params.set("cursor", options.cursor);
  const page = await requestJson<{ items: TransactionDto[]; nextCursor: string | null }>(`/api/transactions?${params}`, { token });
  return { items: page.items.map(toRecord), nextCursor: page.nextCursor };
}

export async function getTransaction(token: string, id: string): Promise<TransactionRecord> {
  const dto = await requestJson<TransactionDto>(`/api/transactions/${encodeURIComponent(id)}`, { token });
  return toRecord(dto);
}

export async function getTransactionSummary(token: string, date = new Date()): Promise<TransactionSummary> {
  const month = `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}`;
  return requestJson<TransactionSummary>(`/api/transactions/summary?month=${month}`, { token });
}
