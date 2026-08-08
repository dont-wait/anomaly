export const API_BASE_URL =
    import.meta.env.VITE_API_URL || "http://localhost:3000";

export const APP_NAME = "Anomaly";

export const TRANSACTION_LIMITS = {
    DAILY_WITHDRAWAL: 50_000_000,
    SINGLE_WITHDRAWAL: 10_000_000,
} as const;
