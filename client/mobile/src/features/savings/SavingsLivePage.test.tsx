import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AuthContext, type AuthContextValue } from "@/features/auth/authContext";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import { getInfo } from "@/features/auth/api/profile";
import { ApiError } from "@/shared/lib/http";
import {
  createSavingsQuote, createSavingsWithdrawalQuote, getSavingsContract, getSavingsHistory,
  getSavingsTerms, listSavingsContracts, listSavingsPackages, openSavingsContract,
  withdrawSavings,
} from "./api";
import SavingsLivePage from "./SavingsLivePage";

vi.mock("@/features/auth/api/profile", async (original) => ({
  ...(await original<typeof import("@/features/auth/api/profile")>()),
  getInfo: vi.fn(),
}));
vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  createSavingsQuote: vi.fn(),
  createSavingsWithdrawalQuote: vi.fn(),
  getSavingsContract: vi.fn(),
  getSavingsHistory: vi.fn(),
  getSavingsTerms: vi.fn(),
  listSavingsContracts: vi.fn(),
  listSavingsPackages: vi.fn(),
  openSavingsContract: vi.fn(),
  withdrawSavings: vi.fn(),
}));

const user = {
  id: "account-1", accountNo: "123456", username: "customer", email: "test@example.com",
  currency: "VND", isVerify: true, amount: 150_000_000,
};
const auth: AuthContextValue = {
  status: AUTH_STATUS.AUTHENTICATED, user, token: "jwt", error: null,
  login: vi.fn(), logout: vi.fn(), refreshProfile: vi.fn(), updateProfile: vi.fn(),
  changePassword: vi.fn(), startEmailChange: vi.fn(), confirmEmailChange: vi.fn(),
  uploadProfileAvatar: vi.fn(), uploadProfileDocument: vi.fn(),
};
const contract = {
  savingsId: "sav-1", savingsNo: "TK-REAL-1", sourceAccountId: "account-1",
  packageId: "savings_standard", principal: 1_000_000, termMonths: 3,
  annualInterestRate: "4.5", openedAt: "2026-01-01T00:00:00Z",
  maturityAt: "2026-04-01T00:00:00Z", status: "active" as const, settlement: null,
};
const quote = {
  quoteId: "quote-1", principal: 10_000_000, currency: "VND", termMonths: 6,
  annualInterestRate: "4.8", expectedMaturityDate: "2027-05-01",
  estimatedInterest: 240_000, estimatedTotalAtMaturity: 10_240_000,
  quoteExpiresAt: "2027-01-01T00:05:00Z", isFinal: false,
};

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
  vi.clearAllMocks();
  vi.mocked(getInfo).mockResolvedValue(user);
  vi.mocked(listSavingsPackages).mockResolvedValue({ items: [{
    packageId: "savings_standard", name: "Tiết kiệm có kỳ hạn", currency: "VND",
    minimumDeposit: 1_000_000,
    termOptions: [
      { termMonths: 3, termLabel: "3 tháng", annualInterestRate: "4.5" },
      { termMonths: 6, termLabel: "6 tháng", annualInterestRate: "4.8" },
    ],
  }] });
  vi.mocked(listSavingsContracts).mockResolvedValue({ items: [contract], nextCursor: null });
  vi.mocked(getSavingsContract).mockResolvedValue(contract);
  vi.mocked(getSavingsTerms).mockResolvedValue({
    version: "savings-standard-v1", title: "Điều kiện gửi tiết kiệm có kỳ hạn",
    introduction: "Các điều kiện dưới đây áp dụng cho sản phẩm Tiết kiệm có kỳ hạn bằng VND tại AnomalyBank.",
    sections: [{ title: "Đáo hạn và tất toán", body: "Không rút trước hạn hoặc rút một phần." }],
  });
  vi.mocked(getSavingsHistory).mockResolvedValue({ items: [] });
  vi.mocked(createSavingsQuote).mockResolvedValue(quote);
  vi.mocked(openSavingsContract).mockResolvedValue({
    savingsId: "sav-new", savingsNo: "TK-REAL-NEW", transactionId: "tx-1",
    principal: 10_000_000, termMonths: 6, annualInterestRate: "4.8", status: "active",
    sourceAccountId: "account-1", openedAt: "2026-11-01T00:00:00Z",
    maturityAt: "2027-05-01T00:00:00Z",
  });
  // The browser supplies UUID v4. Fix it for deterministic assertions.
  Object.defineProperty(globalThis.crypto, "randomUUID", {
    configurable: true, value: () => "58fd795e-b070-46c3-8f9e-9487063b8b27",
  });
});
afterEach(() => cleanup());

function mount() {
  return render(<AuthContext.Provider value={auth}><SavingsLivePage /></AuthContext.Provider>);
}
async function startOpening() {
  mount();
  await screen.findByText("#TK-REAL-1");
  fireEvent.click(screen.getByRole("button", { name: /Gửi tiết kiệm mới/ }));
  fireEvent.change(screen.getByRole("combobox", { name: "Kỳ hạn gửi" }), { target: { value: "6" } });
  fireEvent.click(screen.getByRole("button", { name: /Tiếp tục/ }));
  await screen.findByRole("heading", { name: "Xác nhận gửi tiết kiệm" });
  expect(screen.getByRole("checkbox")).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("button", { name: /Xem phiếu đăng ký & điều khoản/ }));
  expect(screen.getByRole("dialog")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Tôi đã đọc và hiểu" }));
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: /Xác nhận gửi/ }));
}

it("loads real contracts and never displays Stitch mock contracts on authenticated route", async () => {
  mount();
  expect(await screen.findByText("#TK-REAL-1")).toBeTruthy();
  expect(screen.queryByText("#TK-2025-4108")).toBeNull();
  expect(listSavingsContracts).toHaveBeenCalledWith("jwt", undefined, expect.anything());
});

it("restores the original one-screen amount, term and projected-interest layout", async () => {
  mount();
  await screen.findByText("#TK-REAL-1");
  fireEvent.click(screen.getByRole("button", { name: /Gửi tiết kiệm mới/ }));

  expect(screen.getByRole("heading", { name: "Tiết kiệm có kỳ hạn" })).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Số tiền gửi" })).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Chọn kỳ hạn" })).toBeTruthy();
  expect(screen.getByRole("heading", { name: "Ước tính lợi nhuận" })).toBeTruthy();
  expect(screen.getByLabelText("Nhập số tiền (VND)")).toHaveProperty("value", "10000000");
  expect(screen.getByText("Số dư thực tế")).toBeTruthy();
  const term = screen.getByRole("combobox", { name: "Kỳ hạn gửi" });
  expect(term).toBeTruthy();
  // The default term (12 months) is not in this API fixture: force a valid selection.
  expect(screen.getByRole("button", { name: /Tiếp tục/ })).toHaveProperty("disabled", true);

  fireEvent.change(term, { target: { value: "6" } });
  expect(screen.getByText("240.000 VND")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Tiếp tục/ }));
  expect(await screen.findByRole("heading", { name: "Xác nhận gửi tiết kiệm" })).toBeTruthy();
  expect(createSavingsQuote).toHaveBeenCalledWith("jwt", {
    packageId: "savings_standard", sourceAccountId: "account-1",
    amount: 10_000_000, termMonths: 6,
  });
});

it("opens a real savings contract with the backend quote, JWT and stable key", async () => {
  await startOpening();
  expect(await screen.findByText("Gửi tiết kiệm thành công!")).toBeTruthy();
  expect(openSavingsContract).toHaveBeenCalledWith("jwt", {
    quoteId: "quote-1", sourceAccountId: "account-1", acceptedTermsVersion: "savings-standard-v1", confirm: true,
  }, "58fd795e-b070-46c3-8f9e-9487063b8b27");
  expect(localStorage.getItem("anomaly:savings:pending:account-1")).toBeNull();
});

it("retains the original command and key after ambiguous 503, retries without double opening", async () => {
  vi.mocked(openSavingsContract)
    .mockRejectedValueOnce(new ApiError(503, "unknown", { code: "SAVINGS_OPENING_UNAVAILABLE" }));
  await startOpening();
  expect(await screen.findByText(/Yêu cầu giao dịch gần nhất đang được xác minh/)).toBeTruthy();
  expect(localStorage.getItem("anomaly:savings:pending:account-1")).toContain("quote-1");
  fireEvent.click(screen.getByRole("button", { name: "Kiểm tra lại yêu cầu" }));
  await waitFor(() => expect(openSavingsContract).toHaveBeenCalledTimes(2));
  expect(vi.mocked(openSavingsContract).mock.calls[0]).toEqual(vi.mocked(openSavingsContract).mock.calls[1]);
  expect(await screen.findByText("Gửi tiết kiệm thành công!")).toBeTruthy();
});

it("requests a real maturity quote before showing the withdrawal confirmation", async () => {
  const matured = { ...contract, status: "matured" as const };
  vi.mocked(listSavingsContracts).mockResolvedValue({ items: [matured], nextCursor: null });
  vi.mocked(getSavingsContract).mockResolvedValue(matured);
  vi.mocked(createSavingsWithdrawalQuote).mockResolvedValue({
    quoteId: "withdraw-quote", savingsId: "sav-1", withdrawalType: "full",
    principal: 1_000_000, interestAmount: 10_000, totalPayout: 1_010_000,
    maturityAt: matured.maturityAt, destinationAccountId: "account-1",
    quoteExpiresAt: "2026-11-01T10:00:00Z",
  });
  vi.mocked(withdrawSavings).mockResolvedValue({
    savingsId: "sav-1", savingsNo: "TK-REAL-1", transactionId: "tx-withdraw",
    status: "settled", principal: 1_000_000, interestAmount: 10_000,
    totalPayout: 1_010_000, destinationAccountId: "account-1",
    settledAt: "2026-11-01T10:00:00Z",
  });
  mount();
  fireEvent.click(await screen.findByRole("button", { name: /#TK-REAL-1/ }));
  await screen.findByRole("heading", { name: "Chi tiết sổ tiết kiệm" });
  fireEvent.click(screen.getByRole("button", { name: "Rút tiền tiết kiệm" }));
  expect(await screen.findByRole("heading", { name: "Xác nhận rút tiết kiệm" })).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(screen.getByRole("button", { name: "Xác nhận rút tiền" }));
  expect(await screen.findByText("Rút tiết kiệm thành công!")).toBeTruthy();
  expect(withdrawSavings).toHaveBeenCalledWith("jwt", "sav-1", "withdraw-quote", expect.any(String));
});

it("prevents opening unless the customer reviews product terms and consents", async () => {
  mount();
  await screen.findByText("#TK-REAL-1");
  fireEvent.click(screen.getByRole("button", { name: /Gửi tiết kiệm mới/ }));
  fireEvent.change(screen.getByRole("combobox", { name: "Kỳ hạn gửi" }), { target: { value: "6" } });
  fireEvent.click(screen.getByRole("button", { name: /Tiếp tục/ }));
  await screen.findByRole("heading", { name: "Xác nhận gửi tiết kiệm" });
  expect(screen.getByRole("checkbox")).toHaveProperty("disabled", true);
  expect(screen.getByRole("button", { name: /Xác nhận gửi/ })).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("button", { name: /Xem phiếu đăng ký & điều khoản/ }));
  expect(await screen.findByText(/các điều kiện dưới đây áp dụng cho sản phẩm/i)).toBeTruthy();
  expect(screen.queryByText(/mvp|không phải văn bản hợp đồng pháp lý/i)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Đóng" }));
  expect(screen.getByRole("checkbox")).toHaveProperty("disabled", true);
  expect(openSavingsContract).not.toHaveBeenCalled();
});

it("reads the savings ledger through the authenticated history endpoint", async () => {
  vi.mocked(getSavingsHistory).mockResolvedValue({ items: [{
    transactionId: "tx-deposit", type: "savings_open", direction: "debit",
    amount: 1_000_000, balanceAfter: 149_000_000, createdAt: "2026-01-01T00:00:00Z",
  }] });
  mount();
  fireEvent.click(await screen.findByRole("button", { name: /#TK-REAL-1/ }));
  await screen.findByRole("heading", { name: "Chi tiết sổ tiết kiệm" });
  fireEvent.click(screen.getByRole("button", { name: "Xem lịch sử giao dịch" }));
  expect(await screen.findByText("Trích tiền mở sổ")).toBeTruthy();
  expect(getSavingsHistory).toHaveBeenCalledWith("jwt", "sav-1");
});

it("does not submit a deposit if saving the pending command fails", async () => {
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("quota full"); });
  try {
    await startOpening();
    expect(openSavingsContract).not.toHaveBeenCalled();
    expect(await screen.findByText(/Không thể lưu an toàn yêu cầu giao dịch/)).toBeTruthy();
  } finally { spy.mockRestore(); }
});
