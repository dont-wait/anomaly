export const API_ENDPOINTS = {
  AUTH: {
    LOGIN: "/api/auth/login",
    REGISTER: "/api/auth/register",
    ME: "/api/auth/me",
    PROFILE_DOCUMENTS: "/api/auth/me/documents",
    PROFILE_AVATAR: "/api/auth/me/avatar",
    PROFILE_MEDIA: (id: string) => `/api/auth/me/media/${encodeURIComponent(id)}`,
    PROFILE_DOCUMENT: (id: string) => `/api/auth/me/documents/${encodeURIComponent(id)}`,
    OTP_REQUEST: "/api/auth/otp/request",
    OTP_VERIFY: "/api/auth/otp/verify",
  },
  ACCOUNT_LOOKUP: "/api/accounts/lookup",
  KYC: {
    SESSION: "/api/kyc/session",
    COMPLETE: "/api/kyc/complete",
    REALTIME: "/api/kyc/realtime/attempts",
  },
  TRANSFERS: {
    CREATE: "/api/transfers",
    CONFIRM: (id: string) => `/api/transfers/${encodeURIComponent(id)}/confirm`,
    RESEND_OTP: (id: string) =>
      `/api/transfers/${encodeURIComponent(id)}/otp/resend`,
    RECENT_RECIPIENTS: "/api/transfers/recent-recipients",
  },
  CONTACTS: {
    RECIPIENTS: "/api/contacts/recipients",
    DELETE_RECIPIENT: (bankCode: string, accountNo: string) =>
      `/api/contacts/recipients/${encodeURIComponent(bankCode)}/${encodeURIComponent(accountNo)}`,
  },
  TRANSACTIONS: {
    LIST: "/api/transactions",
    DETAIL: (id: string) => `/api/transactions/${encodeURIComponent(id)}`,
    SUMMARY: "/api/transactions/summary",
  },
} as const;
