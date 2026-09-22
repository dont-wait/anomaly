export type TransactionDirection = "in" | "out";
export type TransactionStatus = "success" | "pending" | "failed";
export type TransactionKind =
  "transfer" | "payment" | "bill" | "topup" | "savings" | "salary";

export interface Counterparty {
  name: string;
  accountNo?: string;
  bank?: string;
}

export interface TransactionRecord {
  id: string;
  /** Mã tham chiếu hiển thị cho người dùng, dùng khi tra soát */
  reference: string;
  direction: TransactionDirection;
  status: TransactionStatus;
  kind: TransactionKind;
  amount: number;
  fee: number;
  note: string;
  counterparty: Counterparty;
  createdAt: Date;
  balanceAfter?: number;
}

export type TransactionFilter = "all" | TransactionDirection;
