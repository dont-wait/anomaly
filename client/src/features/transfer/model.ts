export interface Recipient {
  accountNo: string;
  name: string;
  bank: string;
}

/** Tài khoản nguồn — lấy từ hồ sơ người dùng đang đăng nhập */
export interface SourceAccount {
  accountNo: string;
  ownerName: string;
  balance: number;
}

export type TransferStep =
  "recipient" | "amount" | "confirm" | "otp" | "result";

export const MIN_TRANSFER_AMOUNT = 1_000;
export const NOTE_MAX_LENGTH = 100;
export const MAX_OTP_ATTEMPTS = 3;
