export const API_BASE_URL =
  import.meta.env.VITE_API_ENDPOINT || "http://localhost:3000";

console.log("API_BASE_URL:", API_BASE_URL);

export const APP_NAME = "Anomaly";

export const TRANSACTION_LIMITS = {
    DAILY_WITHDRAWAL: 50_000_000,
    SINGLE_WITHDRAWAL: 10_000_000,
} as const;
