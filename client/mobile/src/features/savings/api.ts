/** Live Savings REST contract from server/internal/application/savings. No mock fallback. */
import { ApiError, requestJson } from "@/shared/lib/http";

const root = "/api/savings";
const idPath = (id: string) => `${root}/contracts/${encodeURIComponent(id)}`;
const get = <T>(url: string, token: string, signal?: AbortSignal) =>
  requestJson<T>(url, { token, signal });
const post = <T>(url: string, token: string, body: unknown, key?: string) =>
  requestJson<T>(url, {
    method: "POST", token, body,
    ...(key ? { headers: { "Idempotency-Key": key } } : {}),
  });

export interface SavingsTermsSnapshot {
  version: string;
  title: string;
  introduction: string;
  sections: { title: string; body: string }[];
}
export interface SavingsHistoryItem {
  transactionId: string;
  type: string;
  direction: string;
  amount: number;
  balanceAfter: number;
  createdAt: string;
}
export interface SavingsHistoryResponse { items: SavingsHistoryItem[] }
export interface SavingsTermOption {
  termMonths: number;
  termLabel: string;
  annualInterestRate: string;
}
export interface SavingsPackage {
  packageId: string;
  name: string;
  currency: string;
  termOptions: SavingsTermOption[];
  minimumDeposit: number;
}
export interface SavingsQuote {
  quoteId: string;
  principal: number;
  termMonths: number;
  currency: string;
  annualInterestRate: string;
  expectedMaturityDate: string;
  estimatedInterest: number;
  estimatedTotalAtMaturity: number;
  quoteExpiresAt: string;
  isFinal: boolean;
}
export type BackendSavingsStatus = "active" | "matured" | "settled" | "locked";
export interface SavingsSettlement {
  transactionId: string;
  interestAmount: number;
  totalAmount: number;
  settledAt: string;
}
export interface SavingsRecord {
  savingsId: string;
  savingsNo: string;
  sourceAccountId: string;
  packageId: string;
  principal: number;
  termMonths: number;
  annualInterestRate: string;
  openedAt: string;
  maturityAt: string;
  status: BackendSavingsStatus;
  settlement: SavingsSettlement | null;
}
export interface SavingsOpenResult {
  savingsId: string;
  savingsNo: string;
  transactionId: string;
  status: BackendSavingsStatus;
  principal: number;
  annualInterestRate: string;
  termMonths: number;
  openedAt: string;
  maturityAt: string;
  sourceAccountId: string;
}
export interface SavingsWithdrawalQuote {
  quoteId: string;
  savingsId: string;
  withdrawalType: "full";
  principal: number;
  interestAmount: number;
  totalPayout: number;
  maturityAt: string;
  destinationAccountId: string;
  quoteExpiresAt: string;
}
export interface SavingsWithdrawResult {
  savingsId: string;
  savingsNo: string;
  transactionId: string;
  status: "settled";
  principal: number;
  interestAmount: number;
  totalPayout: number;
  destinationAccountId: string;
  settledAt: string;
}
export interface SavingsReceipt {
  savingsId: string;
  savingsNo: string;
  status: "settled";
  settlement: {
    transactionId: string;
    principal: number;
    interestAmount: number;
    totalAmount: number;
    settledAt: string;
  };
}
export interface SavingsPageResponse {
  items: SavingsRecord[];
  nextCursor: string | null;
}
export const getSavingsTerms = (token: string, signal?: AbortSignal) =>
  get<SavingsTermsSnapshot>(`${root}/terms`, token, signal);
export const getSavingsHistory = (token: string, savingsId: string) =>
  get<SavingsHistoryResponse>(`${idPath(savingsId)}/history`, token);
export const listSavingsPackages = (token: string, signal?: AbortSignal) =>
  get<{ items: SavingsPackage[] }>(`${root}/packages`, token, signal);
export const listSavingsContracts = (
  token: string, cursor?: string, signal?: AbortSignal,
) => {
  const query = new URLSearchParams({ status: "all", limit: "50" });
  if (cursor) query.set("cursor", cursor);
  return get<SavingsPageResponse>(`${root}/contracts?${query}`, token, signal);
};
export const getSavingsContract = (token: string, savingsId: string) =>
  get<SavingsRecord>(idPath(savingsId), token);
export const createSavingsQuote = (
  token: string, input: { packageId: string; sourceAccountId: string; amount: number; termMonths: number },
) => post<SavingsQuote>(`${root}/quotes`, token, input);
export const openSavingsContract = (
  token: string, input: { quoteId: string; sourceAccountId: string; acceptedTermsVersion: string; confirm: true }, key: string,
) => post<SavingsOpenResult>(`${root}/contracts`, token, input, key);
export const createSavingsWithdrawalQuote = (token: string, savingsId: string) =>
  post<SavingsWithdrawalQuote>(`${idPath(savingsId)}/withdrawal-quotes`, token, { withdrawalType: "full" });
export const withdrawSavings = (
  token: string, savingsId: string, quoteId: string, key: string,
) => post<SavingsWithdrawResult>(`${idPath(savingsId)}/withdrawals`, token, { quoteId, confirm: true }, key);
export const getSavingsReceipt = (token: string, savingsId: string) =>
  get<SavingsReceipt>(`${idPath(savingsId)}/settlement`, token);

export function describeSavingsError(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.hasCode("SAVINGS_WRITES_DISABLED")) return "Tạm thời chưa thể mở hoặc tất toán sổ. Không có giao dịch mới được thực hiện.";
    if (error.hasCode("SAVINGS_TERMS_CHANGED")) return "Điều kiện gửi tiết kiệm đã được cập nhật. Vui lòng xem lại nội dung và tạo báo giá mới.";
    if (error.hasCode("QUOTE_EXPIRED") || error.hasCode("QUOTE_NOT_FOUND")) return "Báo giá đã hết hạn hoặc không tồn tại. Vui lòng tạo báo giá mới nếu chưa gửi yêu cầu ghi tiền.";
    if (error.hasCode("SAVINGS_NOT_MATURED")) return "Sổ chưa đến ngày đáo hạn, không thể rút trước hạn.";
    if (error.hasCode("INSUFFICIENT_FUNDS")) return "Số dư tài khoản không đủ để mở sổ.";
    if (error.hasCode("SAVINGS_ALREADY_SETTLED")) return "Sổ này đã được tất toán.";
    if (error.hasCode("SAVINGS_READ_CHANGED")) return "Danh sách sổ đã thay đổi, vui lòng làm mới.";
    if (error.status === 401) return "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";
    if (error.status === 0) return "Không thể xác nhận kết quả do mất kết nối. Đừng tạo giao dịch mới; hãy thử lại cùng yêu cầu.";
    if (error.status >= 500) return "Backend đang không sẵn sàng hoặc kết quả chưa xác định. Không tạo yêu cầu mới; kiểm tra và thử lại cùng khóa.";
    return error.message;
  }
  return error instanceof Error ? error.message : "Không thể xử lý yêu cầu Tiết kiệm.";
}

/** A failed financial command may already have committed: never rotate its key automatically. */
export function mayBeCommitted(error: unknown): boolean {
  return error instanceof ApiError && (error.status === 0 || (error.status >= 500 && !error.hasCode("SAVINGS_WRITES_DISABLED")));
}
