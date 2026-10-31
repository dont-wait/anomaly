/** Savings UI-only models. No network or banking write is performed by this feature. */
export type SavingsStatus = "active" | "matured" | "settled";

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
