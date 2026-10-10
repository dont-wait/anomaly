import { requestJson } from "@/shared/lib/http";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import type {
  TransactionDateRange,
  TransactionDirection,
  TransactionRecord,
} from "@/features/transactions/model";
import {
  toRfc3339End,
  toRfc3339Start,
} from "@/features/transactions/utils/dateRange";

interface TransactionDto extends Omit<
  TransactionRecord,
  "createdAt" | "counterparty"
> {
  createdAt: string;
  counterparty: {
    name: string;
    accountNo?: string;
    bank?: string;
    bankCode?: string;
  };
}

function toRecord(dto: TransactionDto): TransactionRecord {
  return {
    ...dto,
    createdAt: new Date(dto.createdAt),
    counterparty: {
      name: dto.counterparty.name,
      accountNo: dto.counterparty.accountNo,
      bank: dto.counterparty.bank ?? dto.counterparty.bankCode,
    },
  };
}

export interface TransactionPage {
  items: TransactionRecord[];
  nextCursor: string | null;
}

export interface TransactionSummary {
  from: string;
  to: string;
  totalIn: number;
  totalOut: number;
}

export async function listTransactions(
  token: string,
  options: {
    direction?: "all" | TransactionDirection;
    query?: string;
    cursor?: string;
    limit?: number;
    dateRange?: TransactionDateRange;
    signal?: AbortSignal;
  } = {},
): Promise<TransactionPage> {
  const params = new URLSearchParams({
    direction: options.direction ?? "all",
    limit: String(options.limit ?? 20),
  });
  if (options.query?.trim()) params.set("q", options.query.trim());
  if (options.cursor) params.set("cursor", options.cursor);
  if (options.dateRange) {
    params.set("from", toRfc3339Start(options.dateRange.from));
    params.set("to", toRfc3339End(options.dateRange.to));
  }
  const page = await requestJson<{
    items: TransactionDto[];
    nextCursor: string | null;
  }>(`${API_ENDPOINTS.TRANSACTIONS.LIST}?${params}`, {
    token,
    signal: options.signal,
  });
  return { items: page.items.map(toRecord), nextCursor: page.nextCursor };
}

export async function getTransaction(
  token: string,
  id: string,
  options: { signal?: AbortSignal } = {},
): Promise<TransactionRecord> {
  const dto = await requestJson<TransactionDto>(
    API_ENDPOINTS.TRANSACTIONS.DETAIL(id),
    {
      token,
      signal: options.signal,
    },
  );
  return toRecord(dto);
}

export async function getTransactionSummary(
  token: string,
  dateRange: TransactionDateRange,
  options: { signal?: AbortSignal } = {},
): Promise<TransactionSummary> {
  const params = new URLSearchParams({
    from: toRfc3339Start(dateRange.from),
    to: toRfc3339End(dateRange.to),
  });
  return requestJson<TransactionSummary>(
    `${API_ENDPOINTS.TRANSACTIONS.SUMMARY}?${params}`,
    {
      token,
      signal: options.signal,
    },
  );
}
