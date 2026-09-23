export const API_ENDPOINTS = {
  AUTH: {
    LOGIN: "/api/auth/login",
    REGISTER: "/api/auth/register",
    ME: "/api/auth/me",
    OTP_REQUEST: "/api/auth/otp/request",
    OTP_VERIFY: "/api/auth/otp/verify",
  },
  MEDIA: {
    UPLOAD: "/api/media/upload",
  },
  ACCOUNTS: {
    VERIFY: (id: string) => `/api/accounts/${encodeURIComponent(id)}/verify`,
  },
  KYC: {
    VERIFY_FACE: "/v1/kyc/verify-face",
  },
} as const;
