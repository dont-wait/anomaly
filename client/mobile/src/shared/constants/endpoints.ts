export const API_ENDPOINTS = {
  AUTH: {
    LOGIN: "/api/auth/login",
    REGISTER: "/api/auth/register",
    ME: "/api/auth/me",
    OTP_REQUEST: "/api/auth/otp/request",
    OTP_VERIFY: "/api/auth/otp/verify",
  },
  ACCOUNT_LOOKUP: "/api/accounts/lookup",
  KYC: {
    SESSION: "/api/kyc/session",
    COMPLETE: "/api/kyc/complete",
  },
  TRANSFERS: {
    CREATE: "/api/transfers",
    CONFIRM: (id: string) =>
      `/api/transfers/${encodeURIComponent(id)}/confirm`,
    RESEND_OTP: (id: string) =>
      `/api/transfers/${encodeURIComponent(id)}/otp/resend`,
    RECENT_RECIPIENTS: "/api/transfers/recent-recipients",
  },
  TRANSACTIONS: {
    LIST: "/api/transactions",
    DETAIL: (id: string) => `/api/transactions/${encodeURIComponent(id)}`,
    SUMMARY: "/api/transactions/summary",
  },
} as const;
