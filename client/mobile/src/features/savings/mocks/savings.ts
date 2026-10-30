import type { SavingsContract } from "../model";

/** Demo-only fixtures inspired by Stitch S01/S05; never read from or write to a real account. */
export const DEMO_SOURCE_ACCOUNT = "1234 5678 9012";
export const DEMO_ACCOUNT_BALANCE = 68_450_000;
export const demoSavings: readonly SavingsContract[] = [
  {
    id: "demo-active-1",
    number: "TK-2026-9921",
    principal: 10_000_000,
    termMonths: 12,
    annualRate: 5.2,
    openedAt: "2026-10-29T00:00:00.000Z",
    maturityAt: "2027-10-29T00:00:00.000Z",
    status: "active",
    sourceAccount: DEMO_SOURCE_ACCOUNT,
    interest: 520_000,
  },
  {
    id: "demo-matured-2",
    number: "TK-2025-4108",
    principal: 20_000_000,
    termMonths: 12,
    annualRate: 5.2,
    openedAt: "2025-10-29T00:00:00.000Z",
    maturityAt: "2026-10-29T00:00:00.000Z",
    status: "matured",
    sourceAccount: DEMO_SOURCE_ACCOUNT,
    interest: 1_040_000,
  },
];
