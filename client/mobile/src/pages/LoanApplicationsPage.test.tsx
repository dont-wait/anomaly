import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { routes } from "@/app/routes";
import LoanApplicationsPage from "./LoanApplicationsPage";
import {
  attachLoanProfileDocuments,
  confirmLoanContract,
  deleteLoan,
  documentTypes,
  downloadLoanDocument,
  getLoan,
  getLoanOffer,
  listLoans,
  requestLoanOTP,
  uploadLoanDocument,
} from "@/features/loans/api";
import { uploadDocument } from "@/features/auth/api/profileMutations";
import { ApiError } from "@/shared/lib/http";
import type { LoanApplication, LoanOffer } from "@/features/loans/api";

vi.mock("@/features/auth/useAuth", () => ({
  useAuth: () => ({ token: "token", user: { id: "acc-1" } }),
}));

vi.mock("@/features/auth/api/profileMutations", () => ({
  uploadDocument: vi.fn(async () => ({})),
}));

vi.mock("@/features/loans/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/features/loans/api")>();
  return {
    ...actual,
    attachLoanProfileDocuments: vi.fn(),
    deleteLoan: vi.fn(),
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
    vi.mocked(attachLoanProfileDocuments).mockReset();
    vi.mocked(attachLoanProfileDocuments).mockRejectedValue(new Error("none"));
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
    fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));

    expect(await screen.findByText("APP-002")).toBeTruthy();
    await waitFor(() =>
      expect(listLoans).toHaveBeenNthCalledWith(2, "token", "cursor-1"),
    );
    expect(screen.getAllByText("APP-001")).toHaveLength(1);
    expect(screen.queryByRole("navigation", { name: "Phân trang" })).toBeNull();
  });

  it("shows friendly status, amount and an empty state", async () => {
    vi.mocked(listLoans).mockResolvedValueOnce({
      items: [baseApplication],
      nextCursor: null,
    });
    const { unmount } = render(<LoanApplicationsPage />);

    const row = (await screen.findByText("APP-001")).closest("button")!;
    expect(row.textContent).toContain("Đã duyệt");
    expect(row.textContent).toContain("50.000.000");
    expect(row.textContent).not.toContain("approved");
    unmount();

    vi.mocked(listLoans).mockResolvedValueOnce({ items: [], nextCursor: null });
    render(<LoanApplicationsPage />);
    expect(screen.queryByText("Chưa có hồ sơ")).toBeNull();
    expect(await screen.findByText("Chưa có hồ sơ")).toBeTruthy();
  });

  it("creates new applications from the navigation", async () => {
    window.location.hash = "";
    vi.mocked(listLoans).mockResolvedValueOnce({ items: [], nextCursor: null });
    render(<LoanApplicationsPage />);
    await screen.findByText("Chưa có hồ sơ");

    expect(screen.queryByRole("button", { name: "Đăng ký vay" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Tạo mới hồ sơ vay" }));

    expect(window.location.hash).toBe(routes.loanRegister);
  });

  it("guides a draft to completion and uploads from camera or file", async () => {
    const draft = { ...baseApplication, status: "draft" as const };
    vi.mocked(getLoan).mockResolvedValue(draft);
    vi.mocked(uploadLoanDocument).mockResolvedValue(undefined as never);
    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("Đã có 0/7 chứng từ")).toBeTruthy();
    expect(screen.getByText(/Còn thiếu: CCCD mặt trước/)).toBeTruthy();
    const submit = screen.getByRole<HTMLButtonElement>("button", {
      name: "Gửi hồ sơ thẩm định",
    });
    expect(submit.disabled).toBe(true);

    const camera = screen.getByLabelText("CCCD mặt trước: mở camera");
    expect(camera.getAttribute("capture")).toBe("environment");
    expect(camera.getAttribute("accept")).toBe("image/*");
    // Input sr-only phải nằm trong nút, nếu không focus sẽ cuộn nhầm khung overlay.
    expect(camera.closest("label")?.className).toContain("relative");
    const photo = new File(["x"], "front.jpg", { type: "image/jpeg" });
    fireEvent.change(camera, { target: { files: [photo] } });

    await waitFor(() =>
      expect(uploadLoanDocument).toHaveBeenCalledWith(
        "token",
        "app-1",
        "IDENTITY_FRONT",
        photo,
      ),
    );
  });

  it("marks uploaded documents done and asks to redo rejected ones", async () => {
    const doc = {
      uploadedAt: "2026-10-30T00:00:00Z",
      mimeType: "image/jpeg",
      sizeBytes: 1,
    };
    vi.mocked(getLoan).mockResolvedValue({
      ...baseApplication,
      status: "draft",
      documents: [
        {
          ...doc,
          documentId: "d1",
          type: "IDENTITY_FRONT",
          verificationStatus: "pending",
        },
        {
          ...doc,
          documentId: "d2",
          type: "IDENTITY_BACK",
          verificationStatus: "rejected",
        },
      ],
    });
    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("Đã có 1/7 chứng từ")).toBeTruthy();
    expect(screen.queryByLabelText("CCCD mặt trước: mở camera")).toBeNull();
    expect(screen.getByLabelText("CCCD mặt sau: mở camera")).toBeTruthy();
    expect(screen.getByText(/Giấy tờ trước chưa đạt/)).toBeTruthy();
  });

  it("lists submitted documents with status, date and a download action", async () => {
    vi.mocked(getLoan).mockResolvedValue({
      ...baseApplication,
      status: "submitted",
      documents: [
        {
          documentId: "d1",
          type: "IDENTITY_FRONT",
          verificationStatus: "pending",
          uploadedAt: "2026-10-30T12:00:00Z",
          mimeType: "image/jpeg",
          sizeBytes: 1,
        },
      ],
    });
    vi.mocked(downloadLoanDocument).mockRejectedValue(new Error("Lỗi tải"));
    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("Chứng từ đã gửi")).toBeTruthy();
    expect(screen.getByText("Đang chờ xác minh")).toBeTruthy();
    expect(screen.getByText(/^Gửi /)).toBeTruthy();
    fireEvent.click(
      screen.getByRole("button", { name: "Tải xuống CCCD mặt trước" }),
    );
    await waitFor(() =>
      expect(downloadLoanDocument).toHaveBeenCalledWith("token", "app-1", "d1"),
    );
  });

  it("summarises documents by review state and hides uploaded ones", async () => {
    const docs = documentTypes.map((type, index) => ({
      documentId: `d${index}`,
      type,
      verificationStatus: "pending",
      uploadedAt: "2026-10-30T00:00:00Z",
      mimeType: "image/jpeg",
      sizeBytes: 2048,
    }));
    vi.mocked(getLoan).mockResolvedValueOnce({
      ...baseApplication,
      status: "draft",
      documents: docs.slice(1),
    });
    const first = render(<LoanApplicationsPage applicationId="app-1" />);
    expect(await screen.findByText("Vui lòng cập nhật chứng từ")).toBeTruthy();
    expect(screen.getByLabelText("CCCD mặt trước: mở camera")).toBeTruthy();
    expect(screen.queryByLabelText("CCCD mặt sau: mở camera")).toBeNull();
    first.unmount();

    vi.mocked(getLoan).mockResolvedValueOnce({
      ...baseApplication,
      status: "draft",
      documents: docs,
    });
    const second = render(<LoanApplicationsPage applicationId="app-1" />);
    expect(await screen.findByText(/đang chờ ngân hàng duyệt/)).toBeTruthy();
    expect(screen.queryByText("Chứng từ đầy đủ")).toBeNull();
    second.unmount();

    vi.mocked(getLoan).mockResolvedValueOnce({
      ...baseApplication,
      status: "under_review",
      documents: docs.map((doc) => ({
        ...doc,
        verificationStatus: "verified",
      })),
    });
    render(<LoanApplicationsPage applicationId="app-1" />);
    expect(await screen.findByText("Chứng từ đầy đủ")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Tải xuống/ })).toBeNull();
  });

  it("uses identity documents from the profile instead of asking again", async () => {
    const draft = { ...baseApplication, status: "draft" as const };
    vi.mocked(getLoan).mockResolvedValue(draft);
    vi.mocked(attachLoanProfileDocuments).mockResolvedValue({
      ...draft,
      documents: (["IDENTITY_FRONT", "IDENTITY_BACK"] as const).map((type) => ({
        documentId: type,
        type,
        verificationStatus: "pending",
        uploadedAt: "2026-10-30T00:00:00Z",
        mimeType: "image/jpeg",
        sizeBytes: 2048,
      })),
    });
    render(<LoanApplicationsPage applicationId="app-1" />);

    expect(await screen.findByText("Đã có 2/7 chứng từ")).toBeTruthy();
    expect(screen.queryByLabelText("CCCD mặt trước: mở camera")).toBeNull();
    expect(attachLoanProfileDocuments).toHaveBeenCalledTimes(1);
    expect(attachLoanProfileDocuments).toHaveBeenCalledWith("token", "app-1");
  });

  it("stores other documents in the profile, then snapshots them into the application", async () => {
    const draft = { ...baseApplication, status: "draft" as const };
    vi.mocked(getLoan).mockResolvedValue(draft);
    vi.mocked(attachLoanProfileDocuments).mockResolvedValue(draft);
    render(<LoanApplicationsPage applicationId="app-1" />);
    const camera = await screen.findByLabelText(
      "Sao kê lương 3 tháng: mở camera",
    );
    const photo = new File(["x"], "salary.jpg", { type: "image/jpeg" });
    fireEvent.change(camera, { target: { files: [photo] } });

    await waitFor(() =>
      expect(uploadDocument).toHaveBeenCalledWith(
        "token",
        "SALARY_STATEMENT_3_MONTHS",
        photo,
      ),
    );
    await waitFor(() =>
      expect(attachLoanProfileDocuments).toHaveBeenCalledTimes(2),
    );
    expect(uploadLoanDocument).not.toHaveBeenCalled();
  });

  it("deletes only unsent applications after confirmation", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.mocked(deleteLoan).mockResolvedValue(undefined);
    vi.mocked(listLoans).mockResolvedValue({
      items: [
        {
          ...baseApplication,
          id: "draft",
          applicationNo: "APP-D",
          status: "draft",
        },
        baseApplication,
      ],
      nextCursor: null,
    });
    render(<LoanApplicationsPage />);
    await screen.findByText("APP-D");

    expect(
      screen.queryByRole("button", { name: "Xóa hồ sơ APP-001" }),
    ).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Xóa hồ sơ APP-D" }));

    await waitFor(() => expect(screen.queryByText("APP-D")).toBeNull());
    expect(deleteLoan).toHaveBeenCalledWith("token", "draft");
    expect(screen.getByText("APP-001")).toBeTruthy();
  });

  it("pages through applications ten at a time", async () => {
    vi.mocked(listLoans).mockResolvedValue({
      items: Array.from({ length: 12 }, (_, index) => ({
        ...baseApplication,
        id: `a${index}`,
        applicationNo: `NO-${index}`,
      })),
      nextCursor: null,
    });
    render(<LoanApplicationsPage />);
    await screen.findByText("NO-0");
    expect(screen.queryByText("NO-10")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));

    expect(screen.getByText("NO-10")).toBeTruthy();
    expect(screen.queryByText("NO-0")).toBeNull();
    expect(screen.getByText("Trang 2/2")).toBeTruthy();
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

    fireEvent.click(screen.getByRole("button", { name: "Xem đề nghị vay" }));

    expect(await screen.findByText("Chờ bạn phản hồi")).toBeTruthy();
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
      "Hệ thống đang gặp sự cố",
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

    expect(await screen.findByText("Đã chấp nhận")).toBeTruthy();
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
      screen.getByLabelText(
        "Tôi đã đọc và đồng ý các điều khoản của đề nghị vay.",
      ),
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
