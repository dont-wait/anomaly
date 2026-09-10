export type Screen =
  | "email"
  | "document"
  | "profile"
  | "face"
  | "processing"
  | "password"
  | "success"
  | "error";
export const stages: Screen[] = [
  "email",
  "document",
  "profile",
  "face",
  "password",
];
export const titles = [
  "Bắt đầu",
  "Giấy tờ tùy thân",
  "Thông tin cá nhân",
  "Khuôn mặt",
  "Bảo vệ tài khoản",
];
export const passwordRules = (value: string) => [
  value.length >= 8,
  /[a-z]/.test(value) && /[A-Z]/.test(value),
  /\d/.test(value) && /[^a-zA-Z0-9]/.test(value),
];
