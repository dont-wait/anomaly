import type { TransactionDateRange } from "@/features/transactions/model";

const TIME_ZONE = "Asia/Ho_Chi_Minh";

const dateParts = (date: Date) => {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: TIME_ZONE,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).formatToParts(date);
  return {
    year: parts.find((part) => part.type === "year")?.value ?? "",
    month: parts.find((part) => part.type === "month")?.value ?? "",
    day: parts.find((part) => part.type === "day")?.value ?? "",
  };
};

export const dateValueFromDate = (date: Date) => {
  const parts = dateParts(date);
  return `${parts.year}-${parts.month}-${parts.day}`;
};

export const dateValueToDate = (value: string) => {
  const [year, month, day] = value.split("-").map(Number);
  return new Date(Date.UTC(year, month - 1, day, 12));
};

export const formatDateValue = (value: string) =>
  new Intl.DateTimeFormat("vi-VN", {
    timeZone: "UTC",
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(dateValueToDate(value));

export const formatMonthValue = (value: string) =>
  new Intl.DateTimeFormat("vi-VN", {
    timeZone: "UTC",
    month: "long",
    year: "numeric",
  }).format(dateValueToDate(value));

export const startOfMonth = (value: string) => `${value.slice(0, 7)}-01`;

export const addMonths = (value: string, amount: number) => {
  const date = dateValueToDate(value);
  const day = date.getUTCDate();
  const target = new Date(
    Date.UTC(date.getUTCFullYear(), date.getUTCMonth() + amount, 1, 12),
  );
  const lastDay = new Date(
    Date.UTC(target.getUTCFullYear(), target.getUTCMonth() + 1, 0),
  ).getUTCDate();
  target.setUTCDate(Math.min(day, lastDay));
  return dateValueFromDate(target);
};

export const addDays = (value: string, amount: number) => {
  const date = dateValueToDate(value);
  date.setUTCDate(date.getUTCDate() + amount);
  return dateValueFromDate(date);
};

export const currentMonthRange = (date = new Date()): TransactionDateRange => {
  const today = dateValueFromDate(date);
  return { from: startOfMonth(today), to: today };
};

export const allowedTransactionDateRange = (
  date = new Date(),
): TransactionDateRange => {
  const today = dateValueFromDate(date);
  return { from: addMonths(today, -2), to: today };
};

export const toRfc3339Start = (value: string) =>
  `${value}T00:00:00+07:00`;

export const toRfc3339End = (value: string) =>
  `${value}T23:59:59.999999999+07:00`;

export const isDateInRange = (
  value: string,
  range: TransactionDateRange,
) => value >= range.from && value <= range.to;
