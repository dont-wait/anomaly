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
    vi.mocked(getTransactionReport).mockImplementation(async (date) =>
      reportFixture(date),
    );
  });
  afterEach(() => vi.clearAllMocks());

  async function loadPage() {
    render(<TransactionsRiskPage />);
    await screen.findByRole("heading", { name: "Giao dịch & Rủi ro" });
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
    expect(
      screen.getByText("Doanh số thực hiện").closest("article")?.textContent,
    ).not.toContain("700.000");
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
});
