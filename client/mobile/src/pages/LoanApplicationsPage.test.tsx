import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import LoanApplicationsPage from "./LoanApplicationsPage";
import {
  confirmLoanContract,
  getLoan,
  getLoanOffer,
  listLoans,
  requestLoanOTP,
} from "@/features/loans/api";
import { ApiError } from "@/shared/lib/http";
import type { LoanApplication, LoanOffer } from "@/features/loans/api";

vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => ({ token: "token", user: { id: "acc-1" } }),
}));

vi.mock("@/features/loans/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/loans/api")>();
  return {
    ...actual,
    listLoans: vi.fn(),
    getLoan: vi.fn(),
    getLoanOffer: vi.fn(),
    acceptLoanOffer: vi.fn(),
    rejectLoanOffer: vi.fn(),
    requestLoanOTP: vi.fn(),
    confirmLoanContract: vi.fn(),
    submitLoan: vi.fn(),
    uploadLoanDocument: vi.fn(),
    downloadLoanDocument: vi.fn(),
    getApplicationQuote: vi.fn(),
  };
});

const baseApplication: LoanApplication = {
  id: "app-1",
  applicationNo: "APP-001",
  loanPackageId: "pkg-1",
  disbursementAccountId: "acc-1",
  requestedAmount: 50_000_000,
  termMonths: 12,
  status: "approved",
  documents: [],
  createdAt: "2026-10-30T00:00:00Z",
};

const acceptedOffer: LoanOffer = {
  status: "accepted",
  amount: 50_000_000,
  annualRateBps: 1200,
  termMonths: 12,
  issuedAt: "2026-10-30T00:00:00Z",
  acceptedAt: "2026-10-30T00:01:00Z",
};

const confirmedOffer: LoanOffer = {
  ...acceptedOffer,
  status: "confirmed",
  confirmedAt: "2026-10-30T00:02:00Z",
};

describe("LoanApplicationsPage", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.mocked(listLoans).mockReset();
    vi.mocked(getLoan).mockReset();
    vi.mocked(getLoanOffer).mockReset();
    vi.mocked(requestLoanOTP).mockReset();
    vi.mocked(confirmLoanContract).mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("dedupes applications when loading more", async () => {
    vi.mocked(listLoans)
      .mockResolvedValueOnce({
        items: [baseApplication],
        nextCursor: "cursor-1",
      })
      .mockResolvedValueOnce({
        items: [
          baseApplication,
          { ...baseApplication, id: "app-2", applicationNo: "APP-002" },
        ],
        nextCursor: null,
      });

    render(<LoanApplicationsPage />);

    expect(await screen.findByText("APP-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Xem thêm hồ sơ" }));

    expect(await screen.findByText("APP-002")).toBeTruthy();
    await waitFor(() =>
      expect(listLoans).toHaveBeenNthCalledWith(2, "token", "cursor-1"),
    );
    expect(screen.getAllByText("APP-001")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Xem thêm hồ sơ" })).toBeNull();
  });

  it("treats a missing offer as an empty state", async () => {
    vi.mocked(getLoan).mockResolvedValue(baseApplication);
    vi.mocked(getLoanOffer)
      .mockRejectedValueOnce(new ApiError(404, "Not found"))
      .mockResolvedValue({ ...acceptedOffer, status: "issued" });

    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("APP-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Làm mới trạng thái" }));
    await waitFor(() =>
      expect(getLoanOffer).toHaveBeenCalledWith("token", "app-1"),
    );
    expect(screen.queryByRole("alert")).toBeNull();

    fireEvent.click(
      screen.getByRole("button", { name: "Xem Offer đã phát hành" }),
    );

    expect(await screen.findByText("issued")).toBeTruthy();
  });

  it("surfaces non-404 offer errors", async () => {
    vi.mocked(getLoan).mockResolvedValue(baseApplication);
    vi.mocked(getLoanOffer).mockRejectedValue(
      new ApiError(500, "Không tải được Offer."),
    );

    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("APP-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Làm mới trạng thái" }));

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Không tải được Offer.",
    );
  });

  it("confirms a contract with OTP and accepts terms", async () => {
    vi.mocked(getLoan).mockResolvedValue({
      ...baseApplication,
      offer: acceptedOffer,
    });
    vi.mocked(getLoanOffer)
      .mockResolvedValueOnce(acceptedOffer)
      .mockResolvedValue(confirmedOffer);
    vi.mocked(confirmLoanContract).mockResolvedValue(confirmedOffer);

    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("accepted")).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Gửi OTP xác nhận hợp đồng" }),
    );
    await waitFor(() =>
      expect(requestLoanOTP).toHaveBeenCalledWith("token", "app-1"),
    );

    fireEvent.change(screen.getByLabelText("Mã OTP từ email"), {
      target: { value: "123456" },
    });
    fireEvent.click(
      screen.getByLabelText("Tôi đã đọc và đồng ý các điều khoản Offer."),
    );
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận hợp đồng" }));

    await waitFor(() =>
      expect(confirmLoanContract).toHaveBeenCalledWith(
        "token",
        "app-1",
        "123456",
      ),
    );
    expect(
      await screen.findByText(
        "Đã xác nhận hợp đồng bằng OTP. Chờ admin giải ngân.",
      ),
    ).toBeTruthy();
  });
});
