import type { SavingsDraft, SavingsTerm } from "../model";

export const SAVINGS_TERMS: readonly SavingsTerm[] = [
  { months: 3, rate: 4.5 },
  { months: 6, rate: 4.8 },
  { months: 9, rate: 5.0 },
  { months: 12, rate: 5.2 },
  { months: 24, rate: 5.5 },
  { months: 36, rate: 5.8 },
  { months: 60, rate: 6.0 },
];

export const formatVnd = (amount: number) =>
  `${new Intl.NumberFormat("vi-VN", { maximumFractionDigits: 0 }).format(amount)} VND`;

export const formatDate = (iso: string) =>
  new Intl.DateTimeFormat("vi-VN", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(iso));

/** Date-only UTC arithmetic avoids local timezone/DST display drift. */
export function addMonths(isoDate: string, months: number): string {
  const date = new Date(isoDate);
  const targetMonth = date.getUTCMonth() + months;
  const lastDay = new Date(Date.UTC(date.getUTCFullYear(), targetMonth + 1, 0)).getUTCDate();
  const result = new Date(date);
  result.setUTCDate(1);
  result.setUTCMonth(targetMonth);
  result.setUTCDate(Math.min(date.getUTCDate(), lastDay));
  return result.toISOString();
}

/** UI estimate only; the API quote is authoritative once wired up. */
export function estimateSavings(
  amount: number,
  termMonths: number,
  openedAt: string,
): SavingsDraft {
  const term = SAVINGS_TERMS.find((value) => value.months === termMonths);
  if (!term) throw new Error("Kỳ hạn không hợp lệ");
  if (!Number.isSafeInteger(amount) || amount < 1_000_000) {
    throw new Error("Số tiền gửi tối thiểu là 1.000.000 VND");
  }
  const maturityAt = addMonths(openedAt, termMonths);
  const days = (Date.parse(maturityAt) - Date.parse(openedAt)) / 86_400_000;
  // Rounded to VND for a display estimate; exact computation comes from backend.
  const interest = Math.round((amount * term.rate * days) / 36_500);
  return { amount, termMonths, annualRate: term.rate, openedAt, maturityAt, interest };
}

export function parseAmount(input: string): number {
  if (!/^\d*$/.test(input.replace(/[\s.,]/g, ""))) return NaN;
  const normalized = input.replace(/[\s.,]/g, "");
  if (!normalized) return 0;
  const amount = Number(normalized);
  return Number.isSafeInteger(amount) ? amount : NaN;
}
