export interface User {
  id: string;
  name: string;
  email: string;
}

export type TransactionCategory =
  | "food"
  | "shopping"
  | "transfer-in"
  | "transfer-out"
  | "bill"
  | "savings"
  | "other";

export interface Transaction {
  id: string;
  amount: number;
  type: "credit" | "debit";
  description: string;
  date: Date;
  category?: TransactionCategory;
  subtitle?: string;
}

export interface Account {
  id: string;
  ownerName: string;
  accountNumber: string;
  balance: number;
  currency: string;
  cardLabel?: string;
}

export interface QuickAction {
  id: string;
  label: string;
  icon: "transfer" | "topup" | "bill" | "savings" | "more";
  href?: string;
}

export interface PromoBanner {
  id: string;
  title: string;
  subtitle: string;
  ctaLabel: string;
}
