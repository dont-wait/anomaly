import { requestJson } from "@/shared/lib";
import { ANOMALY_BANK, type Bank } from "@/features/transfer/model";

const VIETQR_BASE_URL = "https://api.vietqr.io";

interface VietQrBank {
  code: string;
  bin: string;
  shortName: string;
  name: string;
  logo: string;
  transferSupported: number;
}

interface VietQrBankResponse {
  code: string;
  desc: string;
  data: VietQrBank[];
}

let cache: Promise<Bank[]> | null = null;

/**
 * Danh sách ngân hàng từ VietQR, AnomalyBank luôn đứng đầu.
 * Kết quả được cache trong phiên; lỗi thì xoá cache để lần sau gọi lại.
 */
export function fetchBanks(): Promise<Bank[]> {
  cache ??= requestJson<VietQrBankResponse>("/v2/banks", {
    baseUrl: VIETQR_BASE_URL,
  })
    .then((response) => {
      if (response?.code !== "00" || !Array.isArray(response.data)) {
        throw new Error("Danh sách ngân hàng không hợp lệ");
      }
      const banks = response.data
        .filter((bank) => bank.transferSupported === 1)
        .map(({ code, bin, shortName, name, logo }) => ({
          code,
          bin,
          shortName,
          name,
          logo,
        }));
      return [ANOMALY_BANK, ...banks];
    })
    .catch((error: unknown) => {
      cache = null;
      throw error;
    });
  return cache;
}

export const clearBankCache = () => {
  cache = null;
};
