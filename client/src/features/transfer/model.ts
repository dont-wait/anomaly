import anomalyLogo from "@/assets/logo.png";

export interface Bank {
  /** Mã ngân hàng theo VietQR (vd: VCB), `ANOMALY` cho ngân hàng nội bộ */
  code: string;
  bin: string;
  shortName: string;
  name: string;
  logo: string;
}

export const ANOMALY_BANK: Bank = {
  code: "ANOMALY",
  bin: "",
  shortName: "AnomalyBank",
  name: "Ngân hàng Anomaly",
  logo: anomalyLogo,
};

export interface Recipient {
  accountNo: string;
  name: string;
  bank: Bank;
}

/** Tài khoản nguồn — lấy từ hồ sơ người dùng đang đăng nhập */
export interface SourceAccount {
  accountNo: string;
  ownerName: string;
  balance: number;
}

export const MIN_TRANSFER_AMOUNT = 1_000;
export const NOTE_MAX_LENGTH = 100;
export const MAX_OTP_ATTEMPTS = 3;
export const OTP_LENGTH = 6;
