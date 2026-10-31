import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import LoanContractsPage from "./LoanContractsPage";
import {
  confirmRepayment,
  getLoanContract,
  getRepaymentSchedule,
  listLoanContracts,
  listLoanPayments,
  quoteRepayment,
} from "@/features/loans/api";
import type {
  LoanContract,
  LoanContractDetail,
  RepaymentQuote,
  RepaymentResult,
  RepaymentSchedule,
} from "@/features/loans/api";

vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => ({ token: "token", user: { id: "acc-1" } }),
}));

vi.mock("@/features/loans/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/loans/api")>();
  return {
    ...actual,
    listLoanContracts: vi.fn(),
    getLoanContract: vi.fn(),
    getRepaymentSchedule: vi.fn(),
    listLoanPayments: vi.fn(),
    quoteRepayment: vi.fn(),
    confirmRepayment: vi.fn(),
    getLoanPayment: vi.fn(),
  };
});

const contract: LoanContractDetail = {
  loanId: "loan-1",
  loanNo: "LN-001",
  applicationId: "app-1",
  principal: 50_000_000,
  outstandingPrincipal: 45_000_000,
  annualInterestRate: "12",
  termMonths: 12,
  status: "active",
  paidInstallments: 1,
  totalInstallments: 12,
  nextInstallmentNo: 2,
  nextDueDate: "2026-11-30",
  nextDueAmount: 5_100_000,
  repaymentMethod: "EMI",
  repaymentSchedule: [],
};

const schedule: RepaymentSchedule = {
  loanId: "loan-1",
  outstandingPrincipal: 45_000_000,
  status: "active",
  nextInstallmentNo: 2,
  items: [
    {
      installmentNo: 2,
      dueDate: "2026-11-30",
      principalDue: 4_500_000,
      interestDue: 600_000,
      totalDue: 5_100_000,
      status: "due",
    },
  ],
};

const quote: RepaymentQuote = {
  quoteId: "quote-1",
  loanId: "loan-1",
  installmentNo: 2,
  dueDate: "2026-11-30",
  principalAmount: 4_500_000,
  interestAmount: 600_000,
  penaltyAmount: 0,
  totalAmount: 5_100_000,
  quoteExpiresAt: "2030-01-01T00:00:00Z",
  sourceAccount: { accountId: "acc-1", availableBalance: 9_000_000 },
};

const payment: RepaymentResult = {
  paymentId: "payment-1",
  transactionId: "tx-1",
  loanId: "loan-1",
  installmentNo: 2,
  amount: 5_100_000,
  principalAmount: 4_500_000,
  interestAmount: 600_000,
  penaltyAmount: 0,
  status: "paid",
  paidAt: "2026-11-30T00:00:00Z",
  outstandingPrincipalAfter: 40_500_000,
  loanStatus: "active",
};

const contractForList: LoanContract = {
  ...contract,
  // The list only needs the shared contract fields.
};

describe("LoanContractsPage", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.mocked(listLoanContracts).mockReset();
    vi.mocked(getLoanContract).mockReset();
    vi.mocked(listLoanPayments).mockReset();
    vi.mocked(quoteRepayment).mockReset();
    vi.mocked(confirmRepayment).mockReset();
    vi.mocked(getLoanContract).mockResolvedValue(contract);
    vi.mocked(listLoanPayments).mockResolvedValue({
      items: [],
      nextCursor: null,
    });
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("dedupes contracts when loading more", async () => {
    vi.mocked(listLoanContracts)
      .mockResolvedValueOnce({
        items: [contractForList],
        nextCursor: "cursor-1",
      })
      .mockResolvedValueOnce({
        items: [
          contractForList,
          { ...contractForList, loanId: "loan-2", loanNo: "LN-002" },
        ],
        nextCursor: null,
      });

    render(<LoanContractsPage />);

    expect(await screen.findByText("LN-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Xem thêm hợp đồng" }));

    expect(await screen.findByText("LN-002")).toBeTruthy();
    await waitFor(() =>
      expect(listLoanContracts).toHaveBeenNthCalledWith(2, "token", "cursor-1"),
    );
    expect(screen.getAllByText("LN-001")).toHaveLength(1);
    expect(
      screen.queryByRole("button", { name: "Xem thêm hợp đồng" }),
    ).toBeNull();
  });

  it("keeps the payment result after success and clears it on reload", async () => {
    vi.mocked(getRepaymentSchedule).mockResolvedValue(schedule);
    vi.mocked(quoteRepayment).mockResolvedValue(quote);
    vi.mocked(confirmRepayment).mockResolvedValue(payment);

    render(<LoanContractsPage loanId="loan-1" />);

    expect(await screen.findByText("LN-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Lấy báo giá kỳ 2" }));

    expect(await screen.findByText("Phải trả: 5.100.000 đ")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Xác nhận trả 5.100.000 đ" }),
    );

    await waitFor(() =>
      expect(confirmRepayment).toHaveBeenCalledWith(
        "token",
        "loan-1",
        "quote-1",
        "acc-1",
        expect.any(String),
      ),
    );
    expect(await screen.findByText("Kết quả trả nợ")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Lấy báo giá kỳ 2" }),
    ).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Tải lại dư nợ / lịch sử" }),
    );
    await waitFor(() =>
      expect(screen.queryByText("Kết quả trả nợ")).toBeNull(),
    );
  });

  it("reuses the same idempotency key after a failed confirmation", async () => {
    vi.mocked(getRepaymentSchedule).mockResolvedValue(schedule);
    vi.mocked(quoteRepayment).mockResolvedValue(quote);
    vi.mocked(confirmRepayment)
      .mockRejectedValueOnce(new Error("Mất kết nối."))
      .mockResolvedValueOnce(payment);

    render(<LoanContractsPage loanId="loan-1" />);

    await screen.findByText("LN-001");
    fireEvent.click(screen.getByRole("button", { name: "Lấy báo giá kỳ 2" }));
    await screen.findByText("Phải trả: 5.100.000 đ");

    const confirmButton = screen.getByRole("button", {
      name: "Xác nhận trả 5.100.000 đ",
    });
    fireEvent.click(confirmButton);
    expect((await screen.findByRole("alert")).textContent).toContain(
      "Mất kết nối.",
    );
    fireEvent.click(confirmButton);

    await waitFor(() => expect(confirmRepayment).toHaveBeenCalledTimes(2));
    const firstKey = vi.mocked(confirmRepayment).mock.calls[0][4];
    const secondKey = vi.mocked(confirmRepayment).mock.calls[1][4];
    expect(firstKey).toBe(secondKey);
    expect(await screen.findByText("Kết quả trả nợ")).toBeTruthy();
  });

  it("blocks an expired quote without calling the API", async () => {
    vi.mocked(getRepaymentSchedule).mockResolvedValue(schedule);
    vi.mocked(quoteRepayment).mockResolvedValue({
      ...quote,
      quoteExpiresAt: "2020-01-01T00:00:00Z",
    });

    render(<LoanContractsPage loanId="loan-1" />);

    await screen.findByText("LN-001");
    fireEvent.click(screen.getByRole("button", { name: "Lấy báo giá kỳ 2" }));
    await screen.findByText("Phải trả: 5.100.000 đ");
    fireEvent.click(
      screen.getByRole("button", { name: "Xác nhận trả 5.100.000 đ" }),
    );

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Báo giá đã hết hạn. Vui lòng lấy báo giá mới.",
    );
    expect(confirmRepayment).not.toHaveBeenCalled();
  });
});
