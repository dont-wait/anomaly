export type TimelineCategory =
  "all" | "account" | "transfer" | "otp" | "login" | "loan";

export interface TimelineAccount {
  id: string;
  accountNo: string;
  username: string;
  email: string;
  customerName: string;
  currency: string;
  status: string;
  balance: number;
}

export interface TimelineItem {
  eventId: string;
  eventType: string;
  category: Exclude<TimelineCategory, "all">;
  occurredAt: string;
  sourceStream: string;
  sourceRevision: number;
  accountId?: string;
  actorId?: string;
  transactionId?: string;
  loanId?: string;
  summary: string;
  details?: Record<string, unknown>;
}

export interface TimelineResponse {
  account: TimelineAccount;
  items: TimelineItem[];
  nextCursor?: string;
  hasMore: boolean;
}

export type EventExplorerCategory = "all" | "account" | "transfer" | "otp" | "login" | "loan" | "other";

export interface EventExplorerItem {
  eventId: string;
  eventType: string;
  category: Exclude<EventExplorerCategory, "all">;
  occurredAt: string;
  sourceStream: string;
  sourceRevision: number;
  accounts?: EventExplorerAccount[];
  actorId?: string;
  transactionId?: string;
  loanId?: string;
  summary: string;
  details?: Record<string, unknown>;
}

export interface EventExplorerAccount {
  accountNo: string;
  customerName?: string;
  username?: string;
  role?: string;
}

export interface EventExplorerResponse {
  items: EventExplorerItem[];
  nextCursor?: string;
  hasMore: boolean;
  scanComplete: boolean;
  scanned: number;
}
