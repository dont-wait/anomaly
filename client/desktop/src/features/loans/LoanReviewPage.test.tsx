import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { LoanReviewPage } from "./LoanReviewPage";
import {
  attestLoanEvidence,
  decideLoan,
  disburseLoan,
  getLoan,
  downloadLoanDocument,
  getLoanCustomer,
  getLoanEvidence,
  LoansApiError,
  rejectLoanReview,
  reviewLoanDocument,
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
    getLoanCustomer: vi.fn(),
    getLoanEvidence: vi.fn(),
    rejectLoanReview: vi.fn(),
    reviewLoanDocument: vi.fn(),
    downloadLoanDocument: vi.fn(),
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

const reviewedDocuments = [
  "IDENTITY_FRONT",
  "IDENTITY_BACK",
  "VNEID_PERSONAL",
  "VNEID_RESIDENCE",
  "VSSID_PARTICIPATION",
  "VSSID_DETAIL",
  "SALARY_STATEMENT_3_MONTHS",
].map((type, index) => ({
  documentId: `d${index + 1}`,
  type,
  verificationStatus: "verified",
  uploadedAt: "2026-10-30T00:00:00Z",
  mimeType: "image/jpeg",
  sizeBytes: 2048,
}));
const declared = {
  monthlyIncome: 30_000_000,
  existingMonthlyDebt: 2_000_000,
  regionCode: "KV02",
  purpose: "Mua xe",
};
const tickAllVerifications = () =>
  screen
    .getAllByRole("checkbox")
    .forEach((checkbox) => fireEvent.click(checkbox));
const chooseHistory = () =>
  fireEvent.change(screen.getByLabelText("Lịch sử trả nợ"), {
    target: { value: "previously_late" },
  });

describe("LoanReviewPage", () => {
  beforeEach(() => {
    vi.mocked(listLoans).mockReset();
    vi.mocked(getLoan).mockReset();
    vi.mocked(startLoanReview).mockReset();
    vi.mocked(attestLoanEvidence).mockReset();
    vi.mocked(decideLoan).mockReset();
    vi.mocked(disburseLoan).mockReset();
    // Mặc định bản gốc trùng với danh sách; test riêng sẽ ghi đè khi cần lệch.
    vi.mocked(getLoan).mockImplementation(async (id) => {
      const listed = await vi.mocked(listLoans)();
      return listed.items.find((item) => item.id === id) ?? underReview;
    });
    vi.mocked(getLoanEvidence).mockRejectedValue(
      new LoansApiError("not found", 404),
    );
    vi.mocked(listLoans).mockResolvedValue({ items: [], nextCursor: null });
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

    expect(await screen.findByText("APP-001")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Xem thêm hồ sơ" }));

    expect(await screen.findByText("APP-002")).toBeTruthy();
    await waitFor(() =>
      expect(listLoans).toHaveBeenNthCalledWith(
        2,
        "cursor-1",
        "submitted,under_review,approved",
      ),
    );
    expect(screen.getAllByText("APP-001")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "Xem thêm hồ sơ" })).toBeNull();
  });

  it("shows document status and customer details in tabs without server ids", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        {
          ...underReview,
          customerId: "acc-1",
          documents: [
            {
              documentId: "d1",
              type: "IDENTITY_FRONT",
              verificationStatus: "pending",
              uploadedAt: "2026-10-30T00:00:00Z",
              mimeType: "image/jpeg",
              sizeBytes: 1,
            },
          ],
        },
      ],
      nextCursor: null,
    });
    vi.mocked(getLoanCustomer).mockResolvedValue({
      id: "acc-1",
      accountNo: "1234567890",
      username: "Khách A",
      email: "a@example.com",
      isVerify: true,
      amount: 0,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    expect(screen.queryByText("app-1")).toBeNull();

    fireEvent.click(screen.getByRole("tab", { name: /Chứng từ/ }));
    expect(screen.getByText("Chờ xác minh")).toBeTruthy();
    expect(screen.getAllByText("Khách chưa gửi")).toHaveLength(6);

    fireEvent.click(screen.getByRole("tab", { name: "Khách hàng" }));
    expect(await screen.findByText("Khách A")).toBeTruthy();
    expect(screen.getByText("•••• 7890")).toBeTruthy();
    expect(getLoanCustomer).toHaveBeenCalledWith("acc-1");
  });

  describe("document review", () => {
    const pendingDoc = {
      documentId: "d1",
      type: "IDENTITY_FRONT",
      verificationStatus: "pending",
      uploadedAt: "2026-10-30T00:00:00Z",
      mimeType: "image/jpeg",
      sizeBytes: 1,
    };
    const openDocuments = async (status: LoanApplication["status"]) => {
      vi.mocked(listLoans).mockResolvedValue({
        items: [{ ...underReview, status, documents: [pendingDoc] }],
        nextCursor: null,
      });
      render(<LoanReviewPage />);
      fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
      fireEvent.click(screen.getByRole("tab", { name: /Chứng từ/ }));
    };

    it("approves a document and shows the server result", async () => {
      vi.mocked(reviewLoanDocument).mockResolvedValue({
        ...underReview,
        documents: [{ ...pendingDoc, verificationStatus: "verified" }],
      });
      await openDocuments("under_review");

      fireEvent.click(
        screen.getByRole("button", { name: "Duyệt CCCD mặt trước" }),
      );

      expect(await screen.findByText("Đã xác minh")).toBeTruthy();
      expect(reviewLoanDocument).toHaveBeenCalledWith(
        "app-1",
        "d1",
        "verified",
        "",
      );
    });

    it("requires a reason before rejecting", async () => {
      vi.mocked(reviewLoanDocument).mockResolvedValue(underReview);
      await openDocuments("under_review");

      fireEvent.click(
        screen.getByRole("button", { name: "Từ chối CCCD mặt trước" }),
      );
      const confirm = screen.getByRole<HTMLButtonElement>("button", {
        name: "Xác nhận từ chối",
      });
      expect(confirm.disabled).toBe(true);
      fireEvent.change(screen.getByLabelText(/Lý do từ chối/), {
        target: { value: "Ảnh mờ" },
      });
      fireEvent.click(confirm);

      await waitFor(() =>
        expect(reviewLoanDocument).toHaveBeenCalledWith(
          "app-1",
          "d1",
          "rejected",
          "Ảnh mờ",
        ),
      );
    });

    it("keeps review actions visible but disabled unless under review", async () => {
      await openDocuments("submitted");
      const approve = screen.getByRole<HTMLButtonElement>("button", {
        name: "Duyệt CCCD mặt trước",
      });
      expect(approve.disabled).toBe(true);
      expect(
        screen.getByText(/Chỉ duyệt được khi hồ sơ đang thẩm định/),
      ).toBeTruthy();
    });

    it("previews an image in a dialog and closes with Escape", async () => {
      URL.createObjectURL = vi.fn(() => "blob:preview");
      URL.revokeObjectURL = vi.fn();
      vi.mocked(downloadLoanDocument).mockResolvedValue(new Blob(["x"]));
      await openDocuments("submitted");

      fireEvent.click(
        screen.getByRole("button", { name: "Xem CCCD mặt trước" }),
      );

      const dialog = await screen.findByRole("dialog", {
        name: "Xem CCCD mặt trước",
      });
      expect(within(dialog).getByRole("alert").textContent).toContain(
        "gần như không có nội dung",
      );
      const image = await within(dialog).findByRole("img");
      expect(image.getAttribute("src")).toBe("blob:preview");

      fireEvent.keyDown(window, { key: "Escape" });
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:preview");
    });
  });

  it("references the application's documents as evidence sources", async () => {
    vi.mocked(attestLoanEvidence).mockResolvedValue(undefined as never);
    vi.mocked(listLoans).mockResolvedValue({
      items: [{ ...underReview, declared, documents: reviewedDocuments }],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    chooseHistory();
    tickAllVerifications();
    fireEvent.click(
      screen.getByRole("button", { name: "Lưu kết quả đối soát" }),
    );

    await waitFor(() =>
      expect(attestLoanEvidence).toHaveBeenCalledWith(
        "app-1",
        expect.objectContaining({
          monthlySalaries: [30_000_000, 30_000_000, 30_000_000],
          existingMonthlyDebt: 2_000_000,
          regionCode: "KV02",
          sources: reviewedDocuments.map(
            (doc) => `loan-document:${doc.documentId}`,
          ),
        }),
      ),
    );
  });

  it("blocks evidence until all documents are verified and shows the score after assessment", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        { ...underReview, declared, documents: [] },
        {
          ...underReview,
          id: "app-2",
          applicationNo: "APP-002",
          assessedAt: "2026-11-01T00:00:00Z",
          assessmentScore: 75,
          assessmentDecision: "manual_review",
          assessmentReasons: ["DTI requires manual review"],
        },
      ],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    chooseHistory();
    tickAllVerifications();
    expect(
      screen.getByRole<HTMLButtonElement>("button", {
        name: "Lưu kết quả đối soát",
      }).disabled,
    ).toBe(true);
    expect(screen.getByText(/Khách khai/)).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: /APP-002/ }));
    expect(screen.getByText("Điểm: 75")).toBeTruthy();
    expect(
      screen.getByText("Tỷ lệ nợ trên thu nhập cần xem xét thêm"),
    ).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Chấm điểm" })).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Lưu kết quả đối soát" }),
    ).toBeNull();
  });

  it("starts review for a submitted application", async () => {
    vi.mocked(startLoanReview).mockResolvedValue(underReview);
    vi.mocked(listLoans).mockResolvedValue({
      items: [baseApplication],
      nextCursor: null,
    });

    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.click(screen.getByRole("button", { name: "Bắt đầu thẩm định" }));

    await waitFor(() => expect(startLoanReview).toHaveBeenCalledWith("app-1"));
    expect(await screen.findByText("Đã bắt đầu thẩm định.")).toBeTruthy();
  });

  it("requires the verification checkbox before saving evidence", async () => {
    vi.mocked(attestLoanEvidence).mockResolvedValue(undefined);
    vi.mocked(listLoans).mockResolvedValue({
      items: [{ ...underReview, declared, documents: reviewedDocuments }],
      nextCursor: null,
    });

    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    chooseHistory();

    const evidenceButton = screen.getByRole("button", {
      name: "Lưu kết quả đối soát",
    }) as HTMLButtonElement;
    expect(evidenceButton.disabled).toBe(true);

    tickAllVerifications();
    expect(evidenceButton.disabled).toBe(false);
    fireEvent.click(evidenceButton);

    await waitFor(() =>
      expect(attestLoanEvidence).toHaveBeenCalledWith(
        "app-1",
        expect.objectContaining({
          regionCode: "KV02",
          repaymentHistory: "previously_late",
          insuranceVerified: true,
          documentsVerified: true,
        }),
      ),
    );
    expect(
      await screen.findByText("Đã lưu kết quả đối soát. Tiếp theo: chấm điểm."),
    ).toBeTruthy();
  });

  it("clears the decision reason when switching applications", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        { ...underReview, assessmentDecision: "manual_review" },
        {
          ...underReview,
          id: "app-2",
          applicationNo: "APP-002",
          assessmentDecision: "manual_review",
        },
      ],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.change(screen.getByLabelText(/Lý do/), {
      target: { value: "Lý do của hồ sơ 1" },
    });

    fireEvent.click(screen.getByRole("button", { name: /APP-002/ }));

    expect((screen.getByLabelText(/Lý do/) as HTMLTextAreaElement).value).toBe(
      "",
    );
  });

  it("uses the authoritative application over the stale list row", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [{ ...underReview, status: "approved" }],
      nextCursor: null,
    });
    vi.mocked(getLoan).mockResolvedValue({
      ...underReview,
      status: "approved",
      offer: {
        status: "confirmed",
        amount: 1,
        annualRateBps: 1,
        termMonths: 12,
        issuedAt: "2026-11-01T00:00:00Z",
      },
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    expect(
      await screen.findByRole("button", { name: "Giải ngân" }),
    ).toBeTruthy();
  });

  it("restores saved evidence and only allows assessing after it exists", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [{ ...underReview, documents: reviewedDocuments }],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Chấm điểm" })
        .disabled,
    ).toBe(true);
    await waitFor(() => expect(getLoanEvidence).toHaveBeenCalledWith("app-1"));

    vi.mocked(getLoanEvidence).mockResolvedValue({
      monthlySalaries: [10_000_000, 11_000_000, 12_000_000],
      regionCode: "KV03",
      existingMonthlyDebt: 0,
      insuranceMonths: 24,
      repaymentHistory: "on_time",
      insuranceVerified: true,
      historyVerified: true,
      financialDataVerified: true,
      documentsVerified: true,
      sources: [],
    });
    fireEvent.click(screen.getByRole("button", { name: /APP-001/ }));
    await waitFor(() =>
      expect(
        screen.getByRole<HTMLButtonElement>("button", { name: "Chấm điểm" })
          .disabled,
      ).toBe(false),
    );
    expect(
      (screen.getByLabelText(/Lương tháng 3/) as HTMLInputElement).value,
    ).toBe("12.000.000");
  });

  it("shows the customer's replacement instead of the rejected document", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        {
          ...underReview,
          documents: [
            {
              documentId: "old",
              type: "IDENTITY_FRONT",
              verificationStatus: "rejected",
              rejectionReason: "Ảnh mờ",
              uploadedAt: "2026-10-30T00:00:00Z",
              mimeType: "image/jpeg",
              sizeBytes: 2048,
            },
            {
              documentId: "new",
              type: "IDENTITY_FRONT",
              verificationStatus: "pending",
              uploadedAt: "2026-10-31T00:00:00Z",
              mimeType: "image/jpeg",
              sizeBytes: 2048,
            },
          ],
        },
      ],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.click(screen.getByRole("tab", { name: /Chứng từ/ }));
    expect(screen.getByText("Chờ xác minh")).toBeTruthy();
    expect(screen.queryByText(/Ảnh mờ/)).toBeNull();
  });

  it("rejects an application before assessment with a reason", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.mocked(rejectLoanReview).mockResolvedValue({
      ...underReview,
      status: "rejected",
    });
    vi.mocked(listLoans).mockResolvedValue({
      items: [underReview],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    const reject = screen.getByRole<HTMLButtonElement>("button", {
      name: "Từ chối hồ sơ",
    });
    expect(reject.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/Lý do từ chối/), {
      target: { value: "Chứng từ giả" },
    });
    fireEvent.click(reject);
    await waitFor(() =>
      expect(rejectLoanReview).toHaveBeenCalledWith("app-1", "Chứng từ giả"),
    );
  });

  it("reloads the queue when the status filter changes", async () => {
    vi.mocked(listLoans).mockResolvedValue({ items: [], nextCursor: null });
    render(<LoanReviewPage />);
    await screen.findByText("Chưa có hồ sơ nào.");
    fireEvent.change(screen.getByLabelText("Lọc hồ sơ"), {
      target: { value: "rejected" },
    });
    await waitFor(() =>
      expect(listLoans).toHaveBeenLastCalledWith(undefined, "rejected"),
    );
  });

  it("says when the customer's offer has expired", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        {
          ...underReview,
          status: "approved",
          offer: {
            status: "issued",
            amount: 1,
            annualRateBps: 1,
            termMonths: 12,
            issuedAt: "2020-01-01T00:00:00Z",
            expiresAt: "2020-01-08T00:00:00Z",
          },
        },
      ],
      nextCursor: null,
    });
    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    expect(await screen.findByText(/Đề nghị đã hết hạn/)).toBeTruthy();
  });

  it("submits a manual approval decision", async () => {
    vi.mocked(decideLoan).mockResolvedValue({
      ...underReview,
      status: "approved",
    });
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        {
          ...underReview,
          assessedAt: "2026-11-01T00:00:00Z",
          assessmentDecision: "manual_review",
        },
      ],
      nextCursor: null,
    });

    render(<LoanReviewPage />);
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.change(screen.getByLabelText(/Lý do quyết định thủ công/), {
      target: { value: "Đã đối chiếu đủ chứng từ" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Phê duyệt thủ công" }));

    await waitFor(() =>
      expect(decideLoan).toHaveBeenCalledWith(
        "app-1",
        "approved",
        "Đã đối chiếu đủ chứng từ",
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
    fireEvent.click(await screen.findByRole("button", { name: /APP-001/ }));
    fireEvent.click(screen.getByRole("button", { name: "Giải ngân" }));

    await waitFor(() => expect(disburseLoan).toHaveBeenCalledWith("app-1"));
  });

  it("shows list errors in an alert", async () => {
    vi.mocked(listLoans).mockRejectedValue(new Error("Không tải được hồ sơ."));

    render(<LoanReviewPage />);

    expect((await screen.findByRole("alert")).textContent).toContain(
      "Không tải được hồ sơ.",
    );
  });
});
