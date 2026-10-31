import { act, fireEvent, render, screen } from "@testing-library/react";
import { consumeRiskStream, getMonitorDashboard } from "./api/monitorApi";
import { MonitorPage } from "./MonitorPage";
import { MonitorDashboardSkeleton } from "./components/MonitorDashboardSkeleton";
import { RiskTrendChart } from "./components/RiskTrendChart";
import { routes } from "@/app/routes";

vi.mock("./api/monitorApi", () => ({
  consumeRiskStream: vi.fn(() => new Promise<void>(() => undefined)),
  getMonitorDashboard: vi.fn(),
  shouldAdvanceMonitorCursor: vi.fn(() => true),
}));

vi.mock("react-chartjs-2", () => ({
  Line: (props: { "aria-label"?: string }) => <canvas data-testid="chart-canvas" aria-label={props["aria-label"]} />,
}));

const dashboard = {
  range: "24h" as const,
  updatedAt: "2026-10-28T10:00:00Z",
  dataStatus: "fresh" as const,
  cursor: "cursor-1",
  snapshot: {
    totalAssessments: 2,
    anomalousCount: 1,
    anomalyRate: 0.5,
    averageRiskScore: 0.5533,
    timeseries: [{ timestamp: "2026-10-28T10:00:00Z", assessmentCount: 2, anomalousCount: 1, averageRiskScore: 0.5533 }],
    recentAssessments: [{
      assessmentId: "assessment-1", transactionId: "TX001", riskScore: 0.9821,
      predictedLabel: 1 as const, threshold: 0.9517, modelVersion: "model-v1",
      riskLevel: "BLOCK" as const, decision: "BLOCK" as const,
      featureVersion: "features-v1", receivedAt: "2026-10-28T10:00:00Z",
      transaction: { transactionId: "TX001", sourceAccountNo: "ACC001", sourceAccountName: "Nguyen Van A", destinationAccountNo: "ACC002", destinationName: "Tran Thi B", amount: 50000000, fee: 0, currency: "VND", channel: "mobile", status: "awaiting_otp", createdAt: "2026-10-28T09:59:00Z" },
    }],
    modelVersions: [{ label: "model-v1", count: 2 }],
    normalCount: 0,
    warningCount: 0,
    blockCount: 2,
  },
};

describe("MonitorPage", () => {
  beforeEach(() => {
    vi.mocked(getMonitorDashboard).mockResolvedValue(dashboard);
  });

  afterEach(() => {
    vi.clearAllMocks();
    vi.useRealTimers();
  });

  it("renders model assessment data from the snapshot", async () => {
    render(<MonitorPage />);

    expect(await screen.findByRole("heading", { name: /Giám sát rủi ro/i })).toBeTruthy();
    expect((await screen.findAllByText("TX001")).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("0.9821")).toBeTruthy();
    expect(screen.getAllByText("Chặn").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByRole("heading", { name: "Xu hướng rủi ro (24h)" })).toBeTruthy();
  });

  it("shows only the five most recent requests", async () => {
    const firstAssessment = dashboard.snapshot.recentAssessments[0];
    if (!firstAssessment) throw new Error("Expected a test assessment");
    const assessments = Array.from({ length: 6 }, (_, index) => {
      const transactionId = `TX00${index + 1}`;
      return {
        ...firstAssessment,
        assessmentId: `assessment-${index + 1}`,
        transactionId,
        transaction: firstAssessment.transaction
          ? { ...firstAssessment.transaction, transactionId }
          : null,
      };
    });
    vi.mocked(getMonitorDashboard).mockResolvedValueOnce({
      ...dashboard,
      snapshot: { ...dashboard.snapshot, recentAssessments: assessments },
    });

    render(<MonitorPage />);

    expect(await screen.findByText("TX006")).toBeTruthy();
    expect(screen.getByText("TX002")).toBeTruthy();
    expect(screen.queryByText("TX001")).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "Xem tất cả" }));
    expect(window.location.hash).toBe(routes.audit);
  });

  it("renders the reusable dashboard skeleton", () => {
    const { container } = render(<MonitorDashboardSkeleton />);

    expect(screen.getByRole("status", { name: "Đang tải dashboard" })).toBeTruthy();
    expect(container.querySelectorAll('[data-slot="skeleton"]').length).toBeGreaterThan(10);
  });

  it("shows the dashboard skeleton when the initial request takes longer", async () => {
    vi.useFakeTimers();
    vi.mocked(getMonitorDashboard).mockImplementation(() => new Promise(() => undefined));
    render(<MonitorPage />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(120);
    });

    expect(screen.getByRole("status", { name: "Đang tải dashboard" })).toBeTruthy();
  });

  it("renders an empty state when the risk series has no data", () => {
    render(<RiskTrendChart series={[]} />);

    expect(screen.getByRole("status", { name: "Chưa có dữ liệu" })).toBeTruthy();
    expect(screen.queryByTestId("chart-canvas")).toBeNull();
  });

  it("changes the time range", async () => {
    vi.mocked(getMonitorDashboard).mockResolvedValueOnce(dashboard).mockResolvedValueOnce({ ...dashboard, range: "7d", snapshot: { ...dashboard.snapshot, totalAssessments: 0, anomalousCount: 0, anomalyRate: 0, recentAssessments: [], modelVersions: [], timeseries: [] } });
    render(<MonitorPage />);

    fireEvent.click(screen.getByRole("button", { name: "7 ngày" }));

    expect(await screen.findByText("Chưa có dữ liệu trong khoảng thời gian này.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "7 ngày" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("renders the Chart.js risk trend canvas", async () => {
    render(<MonitorPage />);

    expect(await screen.findByTestId("chart-canvas")).toBeTruthy();
    expect(screen.getByTestId("risk-trend-chart")).toBeTruthy();
  });

  it("refreshes the snapshot", async () => {
    render(<MonitorPage />);
    await screen.findAllByText("TX001");
    fireEvent.click(screen.getByRole("button", { name: "Làm mới" }));

    expect(await screen.findByRole("button", { name: "Làm mới" })).toBeTruthy();
    expect(getMonitorDashboard).toHaveBeenCalled();
    expect(consumeRiskStream).toHaveBeenCalled();
  });
});
