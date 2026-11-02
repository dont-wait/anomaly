/** Shared savings view models for the preview and authenticated API screens. */
export type SavingsStatus = "active" | "matured" | "settled" | "locked";

export interface SavingsContract {
  id: string;
  number: string;
  principal: number;
  termMonths: number;
  annualRate: number;
  openedAt: string;
  maturityAt: string;
  status: SavingsStatus;
  sourceAccount: string;
  interest: number;
  settledAt?: string;
}

export interface SavingsDraft {
  amount: number;
  termMonths: number;
  annualRate: number;
  openedAt: string;
  maturityAt: string;
  interest: number;
}

export interface SavingsTerm {
  months: number;
  rate: number;
}
