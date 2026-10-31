import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LoanReviewPage } from "./LoanReviewPage";
import {
  attestLoanEvidence,
  decideLoan,
  disburseLoan,
  getLoan,
  listLoans,
  startLoanReview,
} from "./api/loansApi";
import type { LoanApplication } from "./model";

vi.mock("./api/loansApi", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./api/loansApi")>();
  return {
    ...actual,
    listLoans: vi.fn(),
    getLoan: vi.fn(),
    startLoanReview: vi.fn(),
    attestLoanEvidence: vi.fn(),
    assessLoan: vi.fn(),
    decideLoan: vi.fn(),
    issueLoanOffer: vi.fn(),
    disburseLoan: vi.fn(),
  };
});

const baseApplication: LoanApplication = {
  id: "app-1",
  applicationNo: "APP-001",
  loanPackageId: "pkg-1",
  disbursementAccountId: "acc-1",
  requestedAmount: 50_000_000,
  termMonths: 12,
  status: "submitted",
  documents: [],
  createdAt: "2026-10-30T00:00:00Z",
};

const underReview: LoanApplication = {
  ...baseApplication,
  status: "under_review",
};

describe("LoanReviewPage", () => {
  beforeEach(() => {
    vi.mocked(listLoans).mockReset();
    vi.mocked(getLoan).mockReset();
    vi.mocked(startLoanReview).mockReset();
    vi.mocked(attestLoanEvidence).mockReset();
    vi.mocked(decideLoan).mockReset();
    vi.mocked(disburseLoan).mockReset();
    vi.mocked(getLoan).mockResolvedValue(underReview);
  });

  afterEach(() => {
    vi.clearAllMocks();
    vi.restoreAllMocks();
  });

  it("loads the admin list and dedupes pagination", async () => {
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

    render(<LoanReviewPage />);
    fireEvent.click(
      screen.getByRole("button", { name: "Tải danh sách hồ sơ" }),
    );

    expect(await screen.findByText("APP-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Xem thêm hồ sơ" }));

    expect(await screen.findByText("APP-002")).toBeTruthy();
    await waitFor(() =>
      expect(listLoans).toHaveBeenNthCalledWith(2, "cursor-1"),
    );
    expect(screen.getAllByText("APP-001")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Xem thêm hồ sơ" })).toBeNull();
  });

  it("opens a trimmed application id", async () => {
    render(<LoanReviewPage />);
    fireEvent.change(screen.getByLabelText("Application ID"), {
      target: { value: " app-1 " },
    });
    fireEvent.click(screen.getByRole("button", { name: "Mở hồ sơ" }));

    expect(await screen.findByText(underReview.applicationNo)).toBeTruthy();
    expect(getLoan).toHaveBeenCalledWith("app-1");
  });

  it("starts review for a submitted application", async () => {
    vi.mocked(startLoanReview).mockResolvedValue(underReview);
    vi.mocked(listLoans).mockResolvedValue({
      items: [baseApplication],
      nextCursor: null,
    });

    render(<LoanReviewPage />);
    fireEvent.click(
      screen.getByRole("button", { name: "Tải danh sách hồ sơ" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.click(screen.getByRole("button", { name: "Bắt đầu review" }));

    await waitFor(() => expect(startLoanReview).toHaveBeenCalledWith("app-1"));
    expect(await screen.findByText("Đã bắt đầu thẩm định.")).toBeTruthy();
  });

  it("requires the verification checkbox before saving evidence", async () => {
    vi.mocked(attestLoanEvidence).mockResolvedValue(undefined);
    vi.mocked(listLoans).mockResolvedValue({
      items: [underReview],
      nextCursor: null,
    });

    render(<LoanReviewPage />);
    fireEvent.click(
      screen.getByRole("button", { name: "Tải danh sách hồ sơ" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));

    const evidenceButton = screen.getByRole("button", {
      name: "Ghi Evidence đã kiểm tra",
    }) as HTMLButtonElement;
    expect(evidenceButton.disabled).toBe(true);

    fireEvent.click(screen.getByRole("checkbox"));
    expect(evidenceButton.disabled).toBe(false);
    fireEvent.click(evidenceButton);

    await waitFor(() =>
      expect(attestLoanEvidence).toHaveBeenCalledWith(
        "app-1",
        expect.objectContaining({
          regionCode: "KV01",
          repaymentHistory: "previously_late",
          insuranceVerified: true,
          documentsVerified: true,
        }),
      ),
    );
    expect(
      await screen.findByText("Đã lưu Evidence. Tiếp theo: chấm điểm."),
    ).toBeTruthy();
  });

  it("submits a manual approval decision", async () => {
    vi.mocked(decideLoan).mockResolvedValue({
      ...underReview,
      status: "approved",
    });
    vi.mocked(listLoans).mockResolvedValue({
      items: [{ ...underReview, assessmentDecision: "manual_review" }],
      nextCursor: null,
    });

    render(<LoanReviewPage />);
    fireEvent.click(
      screen.getByRole("button", { name: "Tải danh sách hồ sơ" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.click(screen.getByRole("button", { name: "Phê duyệt thủ công" }));

    await waitFor(() =>
      expect(decideLoan).toHaveBeenCalledWith(
        "app-1",
        "approved",
        "Đã xác minh hồ sơ test và phê duyệt thủ công",
      ),
    );
  });

  it("disburses a confirmed offer after confirmation", async () => {
    vi.mocked(disburseLoan).mockResolvedValue({
      ...baseApplication,
      status: "disbursed",
    });
    vi.mocked(getLoan).mockResolvedValue({
      ...baseApplication,
      status: "approved",
      offer: {
        status: "confirmed",
        amount: baseApplication.requestedAmount,
        annualRateBps: 1200,
        termMonths: 12,
        issuedAt: "2026-10-30T00:00:00Z",
      },
    });
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        {
          ...baseApplication,
          status: "approved",
          offer: {
            status: "confirmed",
            amount: baseApplication.requestedAmount,
            annualRateBps: 1200,
            termMonths: 12,
            issuedAt: "2026-10-30T00:00:00Z",
          },
        },
      ],
      nextCursor: null,
    });
    vi.spyOn(window, "confirm").mockReturnValue(true);

    render(<LoanReviewPage />);
    fireEvent.click(
      screen.getByRole("button", { name: "Tải danh sách hồ sơ" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.click(screen.getByRole("button", { name: "Giải ngân" }));

    await waitFor(() => expect(disburseLoan).toHaveBeenCalledWith("app-1"));
  });

  it("shows list errors in an alert", async () => {
    vi.mocked(listLoans).mockRejectedValue(new Error("Không tải được hồ sơ."));

    render(<LoanReviewPage />);
    fireEvent.click(
      screen.getByRole("button", { name: "Tải danh sách hồ sơ" }),
    );

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Không tải được hồ sơ.",
    );
  });
});
