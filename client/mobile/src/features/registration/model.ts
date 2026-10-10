export type Screen =
  | "email"
  | "otp"
  | "document"
  | "profile"
  | "face"
  | "processing"
  | "password"
  | "success"
  | "error";
export const stages: Screen[] = [
  "email",
  "otp",
  "document",
  "profile",
  "password",
  "face",
];
export const titles = [
  "Bắt đầu",
  "Xác thực email",
  "Giấy tờ tùy thân",
  "Thông tin cá nhân",
  "Bảo vệ tài khoản",
  "Khuôn mặt",
];
export const OTP_LENGTH = 6;
export const passwordRules = (value: string) => [
  value.length >= 8,
  /[a-z]/.test(value) && /[A-Z]/.test(value),
  /\d/.test(value) && /[^a-zA-Z0-9]/.test(value),
];
