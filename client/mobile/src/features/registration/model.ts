export type Screen =
  | "email"
  | "otp"
  | "document"
  | "profile"
  | "face"
  | "processing"
  | "password"
  | "success"
  | "pending"
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
const MAX_PASSWORD_BYTES = 72;
/** Quy tắc cho mật khẩu mới, gồm giới hạn 72 byte của bcrypt ở server. */
export const isAcceptedNewPassword = (value: string) =>
  passwordRules(value).every(Boolean) &&
  new TextEncoder().encode(value).length <= MAX_PASSWORD_BYTES;
