import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import {
  getTransactionReport,
  type TransactionReport,
} from "./api/transactionsReportApi";
import { TransactionsRiskPage } from "./TransactionsRiskPage";
import { reportFixture, reportItem } from "./testFixtures";

vi.mock("./api/transactionsReportApi", () => ({
  getTransactionReport: vi.fn(),
}));
vi.mock("react-chartjs-2", () => ({
  Bar: ({ data }: { data: unknown }) => (
    <pre data-testid="bar-data">{JSON.stringify(data)}</pre>
  ),
  Doughnut: ({ data }: { data: unknown }) => (
    <pre data-testid="pie-data">{JSON.stringify(data)}</pre>
  ),
}));

describe("TransactionsRiskPage single report contract", () => {
  beforeEach(() => {
    vi.mocked(getTransactionReport).mockReset();
    vi.mocked(getTransactionReport).mockImplementation(async (date) =>
      reportFixture(date),
    );
  });
  afterEach(() => vi.clearAllMocks());

  async function loadPage() {
    render(<TransactionsRiskPage />);
    await screen.findByRole("heading", { name: "Giao dịch & Rủi ro" });
    await screen.findByText("Doanh số thực hiện");
  }

  it("distinguishes missing assessments from real score zero and filters correctly", async () => {
    await loadPage();
    const table = screen.getByRole("table");
    const missing = within(table).getByRole("row", { name: /tx-unassess/ });
    expect(within(missing).getByText("Chưa có đánh giá")).toBeTruthy();
    expect(within(missing).queryByText("Cho phép")).toBeNull();
    expect(within(missing).queryByText("0.0%")).toBeNull();
    const zero = within(table).getByRole("row", { name: /tx-zero/ });
    expect(within(zero).getByText("Cho phép")).toBeTruthy();
    expect(within(zero).getByText("0.0%")).toBeTruthy();
    expect(within(zero).getByText("Hết hạn OTP")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Kết quả rủi ro"), {
      target: { value: "ALLOW" },
    });
    expect(within(table).queryByText(/tx-unassess/)).toBeNull();
    fireEvent.click(within(table).getByText(/tx-zero/));
    const detail = screen.getByLabelText("Chi tiết giao dịch");
    expect(within(detail).getAllByText("0.00%")).toHaveLength(1); // score only; unknown threshold is not zero
    expect(within(detail).getByText(/Assessment quan sát được/)).toBeTruthy();
  });

  it("reads completed revenue separately and includes all created statuses in charts", async () => {
    await loadPage();
    expect(
      screen.getByText("Doanh số thực hiện").closest("article")?.textContent,
    ).toContain("700.000 VND");
    expect(
      screen.getByText("Khởi tạo trong ngày").closest("article")?.textContent,
    ).toContain("2");
    const pie = JSON.parse(screen.getByTestId("pie-data").textContent || "{}");
    expect(pie.datasets[0].data).toEqual([0, 0, 0, 1, 1, 0]);
    expect(screen.getByTestId("chart-reconciliation").textContent).toContain(
      "Khởi tạo: 2/2",
    );
    expect(screen.getByTestId("chart-reconciliation").textContent).toContain(
      "Doanh số: 700.000 VND/700.000 VND",
    );
    fireEvent.click(screen.getByRole("button", { name: "Doanh số" }));
    const bar = JSON.parse(screen.getByTestId("bar-data").textContent || "{}");
    expect(bar.datasets).toHaveLength(1);
    expect(bar.datasets[0].data[12]).toBe(700000);
  });

  it("does not show or export yesterday's data under a newly selected date when loading fails", async () => {
    await loadPage();
    vi.mocked(getTransactionReport).mockRejectedValueOnce(
      new Error("Tải ngày mới thất bại"),
    );
    fireEvent.change(screen.getByLabelText("Ngày báo cáo"), {
      target: { value: "2026-11-07" },
    });
    await screen.findByText("Tải ngày mới thất bại");
    expect(
      within(screen.getByRole("table")).queryByText(/tx-unassess/),
    ).toBeNull();
    expect(
      (screen.getByRole("button", { name: "CSV" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(screen.queryByText("Doanh số thực hiện")).toBeNull();
  });

  it("ignores a slow refresh after switching to another report day", async () => {
    await loadPage();
    let resolveOld!: (value: TransactionReport) => void;
    const oldDate = (screen.getByLabelText("Ngày báo cáo") as HTMLInputElement)
      .value;
    vi.mocked(getTransactionReport).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    );
    fireEvent.click(screen.getByLabelText("Làm mới báo cáo"));
    await waitFor(() => expect(resolveOld).toBeTypeOf("function"));
    const nextDate = oldDate === "2026-11-07" ? "2026-11-06" : "2026-11-07";
    vi.mocked(getTransactionReport).mockImplementation(async (date) => ({
      ...reportFixture(date),
      transactions: [reportItem({ transactionId: "new-day" })],
    }));
    fireEvent.change(screen.getByLabelText("Ngày báo cáo"), {
      target: { value: nextDate },
    });
    await screen.findByText(/new-day/);
    await act(async () => {
      resolveOld(reportFixture(oldDate));
    });
    expect(screen.getByText(/new-day/)).toBeTruthy();
    expect(
      within(screen.getByRole("table")).queryByText(/tx-unassess/),
    ).toBeNull();
  });

  it("keeps the date selector available while loading without displaying fake zero statistics", async () => {
    vi.mocked(getTransactionReport).mockImplementation(
      () => new Promise(() => undefined),
    );
    render(<TransactionsRiskPage />);
    expect(screen.getByLabelText("Ngày báo cáo")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toContain(
      "Đang tải báo cáo",
    );
    expect(screen.queryByText("Doanh số thực hiện")).toBeNull();
    expect(
      (screen.getByRole("button", { name: "CSV" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
  });

  it("keeps stale data visibly marked and blocks export during both failed refresh and retry", async () => {
    await loadPage();
    vi.mocked(getTransactionReport).mockRejectedValueOnce(
      new Error("Mất kết nối"),
    );
    fireEvent.click(screen.getByLabelText("Làm mới báo cáo"));
    await screen.findByText("Mất kết nối");
    expect(screen.getByText(/bản tải trước đó/)).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "CSV" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    expect(
      screen.getByText("Doanh số thực hiện").closest("article")?.textContent,
    ).toContain("700.000");
    const date = (screen.getByLabelText("Ngày báo cáo") as HTMLInputElement)
      .value;
    let resolveRetry!: (report: TransactionReport) => void;
    vi.mocked(getTransactionReport).mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveRetry = resolve;
        }),
    );
    fireEvent.click(screen.getByLabelText("Làm mới báo cáo"));
    await waitFor(() => expect(resolveRetry).toBeTypeOf("function"));
    expect(screen.queryByText("Mất kết nối")).toBeNull();
    expect(screen.getByText(/bản tải trước đó/)).toBeTruthy();
    expect(
      (screen.getByRole("button", { name: "CSV" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    await act(async () => {
      resolveRetry(reportFixture(date));
    });
    expect(screen.queryByText(/bản tải trước đó/)).toBeNull();
    expect(
      (screen.getByRole("button", { name: "CSV" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it("renders and filters assessments past the first 50 transactions without truncation", async () => {
    vi.mocked(getTransactionReport).mockImplementation(async (date) => {
      const data = reportFixture(date);
      const blocked = {
        ...data.transactions[1].selectedAssessment!,
        decision: "BLOCK" as const,
        riskScore: 0.9,
      };
      data.transactions = Array.from({ length: 61 }, (_, i) =>
        reportItem({
          transactionId: `tx-${String(i).padStart(4, "0")}`,
          createdAt: data.from,
          effectiveStatus: i === 60 ? "expired" : "pending",
          selectedAssessment: i === 60 ? blocked : null,
          latestAssessment: i === 60 ? blocked : null,
          assessmentSource: i === 60 ? "observed" : "none",
        }),
      );
      data.summary.created = {
        transactionCount: 61,
        successCount: 0,
        failedCount: 0,
        cancelledCount: 0,
        expiredCount: 1,
        pendingCount: 60,
        unknownCount: 0,
      };
      data.summary.risk = {
        assessedCount: 1,
        unassessedCount: 60,
        averageScore: 0.9,
        coverage: 1 / 61,
        allowCount: 0,
        modelBlockCount: 1,
        unknownDecisionCount: 0,
        actualBlockedCount: 0,
      };
      data.hourly.created[0] = {
        ...data.hourly.created[0],
        count: 61,
        pendingCount: 60,
      };
      return data;
    });
    await loadPage();
    for (let page = 1; page < 7; page++)
      fireEvent.click(screen.getByRole("button", { name: /Trang sau/ }));
    expect(screen.getByText("Trang 7")).toBeTruthy();
    const row = within(screen.getByRole("table")).getByRole("row", {
      name: /tx-0060/,
    });
    expect(within(row).getByText("Chặn")).toBeTruthy();
    expect(within(row).getByText("90.0%")).toBeTruthy();
    expect(within(row).getByText("Hết hạn OTP")).toBeTruthy();
    fireEvent.change(screen.getByLabelText("Kết quả rủi ro"), {
      target: { value: "BLOCK" },
    });
    expect(screen.getByText("Trang 1")).toBeTruthy();
    expect(within(screen.getByRole("table")).getAllByRole("row")).toHaveLength(
      2,
    );
    expect(
      screen.getByText("Khởi tạo trong ngày").closest("article")?.textContent,
    ).toContain("61");
    expect(
      screen.getByText("Thực tế bị chặn").closest("article")?.textContent,
    ).toContain("0");
  });

  it("distinguishes legacy unknown decisions and keeps applied metadata separate from newer observed risk", async () => {
    vi.mocked(getTransactionReport).mockImplementation(async (date) => {
      const data = reportFixture(date);
      const observed = data.transactions[1].selectedAssessment!;
      const applied = {
        ...observed,
        assessmentId: null,
        decision: null,
        riskScore: 0.3,
        modelVersion: "legacy-applied",
        receivedAt: null,
      };
      data.transactions[0] = reportItem({
        transactionId: "legacy",
        createdAt: data.from,
        canonicalStatus: "failed",
        effectiveStatus: "failed",
        failureReason: "anomalous_transfer",
        selectedAssessment: applied,
        appliedRisk: applied,
        latestAssessment: { ...observed, modelVersion: "new-observed" },
        assessmentSource: "applied",
      });
      data.summary.created = {
        ...data.summary.created,
        failedCount: 1,
        pendingCount: 0,
      };
      data.summary.risk = {
        ...data.summary.risk,
        assessedCount: 2,
        unassessedCount: 0,
        averageScore: 0.15,
        coverage: 1,
        unknownDecisionCount: 1,
        actualBlockedCount: 1,
      };
      data.hourly.created[0] = {
        ...data.hourly.created[0],
        failedCount: 1,
        pendingCount: 0,
      };
      return data;
    });
    await loadPage();
    fireEvent.change(screen.getByLabelText("Kết quả rủi ro"), {
      target: { value: "unknown" },
    });
    const row = within(screen.getByRole("table")).getByRole("row", {
      name: /legacy/,
    });
    expect(within(row).getByText("Chưa rõ quyết định")).toBeTruthy();
    fireEvent.click(row);
    const detail = screen.getByLabelText("Chi tiết giao dịch");
    expect(within(detail).getByText("legacy-applied")).toBeTruthy();
    expect(within(detail).queryByText("new-observed")).toBeNull();
    expect(within(detail).getByText("30.00%")).toBeTruthy();
    expect(
      within(detail).getByText("Nhận assessment").parentElement?.textContent,
    ).toContain("—");
    expect(
      within(detail).getByText("Quyết định đã áp dụng cho giao dịch"),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Đóng" }));
    fireEvent.change(screen.getByLabelText("Kết quả rủi ro"), {
      target: { value: "unassessed" },
    });
    expect(within(screen.getByRole("table")).queryByText(/legacy/)).toBeNull();
  });

  it("labels creation, completion and assessment timestamps separately in Vietnam time", async () => {
    vi.mocked(getTransactionReport).mockImplementation(async (date) => {
      const data = reportFixture(date);
      const risk = {
        ...data.transactions[1].selectedAssessment!,
        evaluatedAt: `${date}T17:00:00Z`,
        receivedAt: `${date}T17:00:30Z`,
      };
      data.transactions = [
        reportItem({
          transactionId: "midnight",
          canonicalStatus: "success",
          effectiveStatus: "success",
          createdAt: `${date}T16:59:00Z`,
          completedAt: `${date}T17:01:00Z`,
          completionTimeSource: "posted_at",
          selectedAssessment: risk,
          latestAssessment: risk,
          assessmentSource: "observed",
        }),
      ];
      data.asOf = `${date}T18:00:00Z`;
      data.generatedAt = data.asOf;
      data.summary.created = {
        transactionCount: 1,
        successCount: 1,
        failedCount: 0,
        cancelledCount: 0,
        expiredCount: 0,
        pendingCount: 0,
        unknownCount: 0,
      };
      data.summary.risk = {
        ...data.summary.risk,
        unassessedCount: 0,
        coverage: 1,
      };
      data.hourly.created[0] = {
        ...data.hourly.created[0],
        count: 0,
        expiredCount: 0,
        pendingCount: 0,
      };
      data.hourly.created[23] = {
        ...data.hourly.created[23],
        count: 1,
        successCount: 1,
      };
      return data;
    });
    await loadPage();
    fireEvent.click(
      within(screen.getByRole("table")).getByRole("row", { name: /midnight/ }),
    );
    const detail = screen.getByLabelText("Chi tiết giao dịch");
    expect(
      within(detail).getByText("Khởi tạo").parentElement?.textContent,
    ).toContain("23:59");
    expect(
      within(detail).getByText("Hoàn tất", { selector: "dt" }).parentElement
        ?.textContent,
    ).toContain("00:01");
    expect(
      within(detail).getByText("Hoàn tất", { selector: "dt" }).parentElement
        ?.textContent,
    ).toContain("mốc ghi sổ lịch sử");
    expect(
      within(detail).getByText("Đánh giá lúc").parentElement?.textContent,
    ).toContain("00:00");
    expect(
      within(detail).getByText("Nhận assessment").parentElement?.textContent,
    ).toContain("00:00");
  });
});
