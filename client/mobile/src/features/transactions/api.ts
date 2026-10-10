import { requestJson } from "@/shared/lib/http";
import { API_ENDPOINTS } from "@/shared/constants/endpoints";
import type {
  TransactionDirection,
  TransactionRecord,
} from "@/features/transactions/model";

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
  month: string;
  totalIn: number;
  totalOut: number;
}

function transactionMonth(date: Date): string {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: "Asia/Ho_Chi_Minh",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(date);
  const year = parts.find((part) => part.type === "year")?.value;
  const month = parts.find((part) => part.type === "month")?.value;
  return `${year}-${month}`;
}

export async function listTransactions(
  token: string,
  options: {
    direction?: "all" | TransactionDirection;
    query?: string;
    cursor?: string;
    limit?: number;
    signal?: AbortSignal;
  } = {},
): Promise<TransactionPage> {
  const params = new URLSearchParams({
    direction: options.direction ?? "all",
    limit: String(options.limit ?? 20),
  });
  if (options.query?.trim()) params.set("q", options.query.trim());
  if (options.cursor) params.set("cursor", options.cursor);
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
  date = new Date(),
  options: { signal?: AbortSignal } = {},
): Promise<TransactionSummary> {
  const month = transactionMonth(date);
  const params = new URLSearchParams({ month });
  return requestJson<TransactionSummary>(
    `${API_ENDPOINTS.TRANSACTIONS.SUMMARY}?${params}`,
    {
      token,
      signal: options.signal,
    },
  );
}
