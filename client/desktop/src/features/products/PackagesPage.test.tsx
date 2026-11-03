import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { PackagesPage } from "./PackagesPage";
import {
  createDraft,
  createPackage,
  deletePackage,
  getPackage,
  listPackages,
  listTerms,
  publishPackage,
  retirePackage,
  updateDraft,
} from "./api";
import { LoansApiError } from "@/features/loans/api/loansApi";
import type { PackageDetail, PackageVersion } from "./model";

vi.mock("./api", async (original) => ({
  ...(await original<typeof import("./api")>()),
  listPackages: vi.fn(),
  getPackage: vi.fn(),
  createDraft: vi.fn(),
  createPackage: vi.fn(),
  deletePackage: vi.fn(),
  updateDraft: vi.fn(),
  deleteDraft: vi.fn(),
  publishPackage: vi.fn(),
  retirePackage: vi.fn(),
  simulateLoan: vi.fn(),
  simulateSavings: vi.fn(),
  listTerms: vi.fn(async () => []),
}));

const active: PackageVersion = {
  kind: "savings",
  code: "savings_standard",
  version: 1,
  status: "ACTIVE",
  name: "Tiết kiệm có kỳ hạn",
  description: "",
  minAmount: 1_000_000,
  terms: [{ months: 12, annualRateBps: 520 }],
};
const summary = {
  kind: "savings" as const,
  code: active.code,
  name: active.name,
  status: "ACTIVE" as const,
  activeVersion: 1,
  minAmount: active.minAmount,
  terms: active.terms,
};
const detail = (versions: PackageVersion[]): PackageDetail => ({
  ...summary,
  versions,
  usage: { active: 3 },
  audit: [],
});
const draft: PackageVersion = {
  ...active,
  version: 2,
  status: "DRAFT",
  terms: [{ months: 12, annualRateBps: 525 }],
};

describe("PackagesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.mocked(listPackages).mockResolvedValue([summary]);
    vi.mocked(getPackage).mockResolvedValue(detail([active]));
  });

  it("shows the package, its rates and usage", async () => {
    render(<PackagesPage kind="savings" />);
    expect(await screen.findByText("12 tháng: 5,2%")).toBeTruthy();
    expect(screen.getByText("Đang gửi: 3")).toBeTruthy();
  });

  it("creates a draft from the active version", async () => {
    vi.mocked(createDraft).mockResolvedValue(draft);
    render(<PackagesPage kind="savings" />);
    fireEvent.click(await screen.findByRole("button", { name: "Sửa gói" }));
    await waitFor(() =>
      expect(createDraft).toHaveBeenCalledWith("savings", "savings_standard"),
    );
  });

  it("saves rates as basis points without floating point drift", async () => {
    vi.mocked(getPackage).mockResolvedValue(detail([draft, active]));
    vi.mocked(updateDraft).mockResolvedValue(draft);
    render(<PackagesPage kind="savings" />);
    fireEvent.change(await screen.findByLabelText("Lãi suất kỳ hạn 12 tháng"), {
      target: { value: "5,29" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Lưu bản nháp" }));
    await waitFor(() =>
      expect(updateDraft).toHaveBeenCalledWith(
        "savings",
        "savings_standard",
        expect.objectContaining({
          terms: [{ months: 12, annualRateBps: 529 }],
        }),
      ),
    );
  });

  it("blocks saving a rate above 100 percent", async () => {
    vi.mocked(getPackage).mockResolvedValue(detail([draft, active]));
    render(<PackagesPage kind="savings" />);
    fireEvent.change(await screen.findByLabelText("Lãi suất kỳ hạn 12 tháng"), {
      target: { value: "101" },
    });
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Lưu bản nháp" })
        .disabled,
    ).toBe(true);
  });

  it("shows what changes and reuses the key when retrying a failed publish", async () => {
    vi.mocked(getPackage).mockResolvedValue(detail([draft, active]));
    vi.mocked(publishPackage)
      .mockRejectedValueOnce(new LoansApiError("timeout", 0))
      .mockResolvedValueOnce({ ...draft, status: "ACTIVE" });
    render(<PackagesPage kind="savings" />);
    expect(await screen.findByText("12 tháng: 5,25%")).toBeTruthy();

    fireEvent.click(screen.getByRole("button", { name: "Phát hành" }));
    await waitFor(() => expect(publishPackage).toHaveBeenCalledTimes(1));
    fireEvent.click(await screen.findByRole("button", { name: "Phát hành" }));
    await waitFor(() => expect(publishPackage).toHaveBeenCalledTimes(2));

    const [first, second] = vi.mocked(publishPackage).mock.calls;
    expect(first[3]).toBe(second[3]);
    expect(first[2]).toBe(2);
  });

  it("explains a version conflict", async () => {
    vi.mocked(getPackage).mockResolvedValue(detail([draft, active]));
    vi.mocked(publishPackage).mockRejectedValue(
      new LoansApiError("conflict", 409, ["PACKAGE_VERSION_CONFLICT"]),
    );
    render(<PackagesPage kind="savings" />);
    fireEvent.click(await screen.findByRole("button", { name: "Phát hành" }));
    expect(
      await screen.findByText(/vừa được người khác thay đổi/),
    ).toBeTruthy();
  });

  it("requires a reason to retire", async () => {
    vi.mocked(retirePackage).mockResolvedValue({
      ...active,
      status: "RETIRED",
    });
    render(<PackagesPage kind="savings" />);
    const retire = await screen.findByRole<HTMLButtonElement>("button", {
      name: "Ngừng bán",
    });
    expect(retire.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Lý do ngừng bán"), {
      target: { value: "Hết chương trình" },
    });
    fireEvent.click(retire);
    await waitFor(() =>
      expect(retirePackage).toHaveBeenCalledWith(
        "savings",
        "savings_standard",
        1,
        "Hết chương trình",
        expect.any(String),
      ),
    );
  });

  it("creates a new package as a draft and opens it", async () => {
    vi.mocked(createPackage).mockResolvedValue({ ...draft, code: "TK_VIP" });
    render(<PackagesPage kind="savings" />);
    fireEvent.click(await screen.findByRole("button", { name: "Tạo gói mới" }));
    const submit = screen.getByRole("button", { name: "Tạo gói" });
    expect(submit).toHaveProperty("disabled", true);
    fireEvent.change(screen.getByLabelText("Mã gói"), {
      target: { value: "tk_vip" },
    });
    fireEvent.change(screen.getByLabelText("Tên gói"), {
      target: { value: "Tiết kiệm VIP" },
    });
    fireEvent.click(screen.getByLabelText("12 tháng"));
    fireEvent.change(screen.getByLabelText("Lãi suất kỳ hạn 12 tháng"), {
      target: { value: "6,5" },
    });
    fireEvent.click(submit);
    await waitFor(() =>
      expect(createPackage).toHaveBeenCalledWith("savings", {
        code: "TK_VIP",
        name: "Tiết kiệm VIP",
        description: "",
        minAmount: 1_000_000,
        maxAmount: 0,
        terms: [{ months: 12, annualRateBps: 650 }],
      }),
    );
    await waitFor(() =>
      expect(getPackage).toHaveBeenCalledWith("savings", "TK_VIP"),
    );
  });

  it("blocks deleting a package on sale or already used", async () => {
    render(<PackagesPage kind="savings" />);
    expect(
      await screen.findByRole("button", { name: "Xóa gói" }),
    ).toHaveProperty("disabled", true);
    expect(
      screen.getByText("Gói đang bán. Muốn xóa, hãy ngừng bán trước."),
    ).toBeTruthy();
  });

  it("deletes an unused retired package", async () => {
    vi.spyOn(window, "confirm").mockReturnValue(true);
    vi.mocked(deletePackage).mockResolvedValue(null);
    vi.mocked(getPackage).mockResolvedValue({
      ...detail([{ ...active, status: "RETIRED" }]),
      usage: {},
    });
    render(<PackagesPage kind="savings" />);
    fireEvent.click(await screen.findByRole("button", { name: "Xóa gói" }));
    await waitFor(() =>
      expect(deletePackage).toHaveBeenCalledWith("savings", "savings_standard"),
    );
    await waitFor(() => expect(listPackages).toHaveBeenCalledTimes(2));
  });

  it("blocks publishing while the form has unsaved changes", async () => {
    vi.mocked(getPackage).mockResolvedValue(detail([draft, active]));
    render(<PackagesPage kind="savings" />);
    const publish = await screen.findByRole("button", { name: "Phát hành" });
    expect(publish).toHaveProperty("disabled", false);
    const rate = screen.getByLabelText("Lãi suất kỳ hạn 12 tháng");
    fireEvent.change(rate, { target: { value: "6" } });
    expect(publish).toHaveProperty("disabled", true);
    expect(screen.getByText(/Có thay đổi chưa lưu/)).toBeTruthy();
    fireEvent.change(rate, { target: { value: "5.25" } });
    expect(publish).toHaveProperty("disabled", false);
  });

  it("blocks publishing terms with unsaved edits", async () => {
    vi.mocked(listTerms).mockResolvedValueOnce([
      {
        version: "v2",
        status: "DRAFT",
        title: "Điều khoản",
        introduction: "Mở đầu",
        sections: [{ title: "Mục", body: "Nội dung" }],
      },
    ]);
    render(<PackagesPage kind="savings" />);
    fireEvent.click(await screen.findByRole("tab", { name: "Điều khoản" }));
    const publish = await screen.findByRole("button", { name: "Phát hành" });
    expect(publish).toHaveProperty("disabled", false);
    fireEvent.change(screen.getByLabelText("Tiêu đề"), {
      target: { value: "Đã sửa" },
    });
    expect(publish).toHaveProperty("disabled", true);
  });

  it("pages through the package list", async () => {
    vi.mocked(listPackages).mockResolvedValue(
      Array.from({ length: 11 }, (_, index) => ({
        ...summary,
        code: `P${index}`,
        name: `Gói ${index}`,
      })),
    );
    render(<PackagesPage kind="savings" />);
    expect(await screen.findByText("Gói 9")).toBeTruthy();
    expect(screen.queryByText("Gói 10")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Trang sau" }));
    expect(screen.getByText("Gói 10")).toBeTruthy();
    expect(screen.queryByText("Gói 0")).toBeNull();
  });

  it("only offers terms on the savings page", async () => {
    vi.mocked(listPackages).mockResolvedValue([]);
    const { unmount } = render(<PackagesPage kind="loan" />);
    await screen.findByText("Chưa có gói nào.");
    expect(screen.queryByRole("tab", { name: "Điều khoản" })).toBeNull();
    unmount();
    render(<PackagesPage kind="savings" />);
    expect(await screen.findByRole("tab", { name: "Điều khoản" })).toBeTruthy();
  });
});
