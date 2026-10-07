import { act, fireEvent, render, screen } from "@testing-library/react";
import { setMockMonitorFailure } from "./api/monitorApi";
import { MonitorPage } from "./MonitorPage";

vi.mock("react-chartjs-2", () => ({
  Line: (props: { "aria-label"?: string }) => <canvas data-testid="chart-canvas" aria-label={props["aria-label"]} />,
}));

describe("MonitorPage", () => {
  afterEach(() => {
    setMockMonitorFailure(false);
    vi.useRealTimers();
  });

  it("renders the Stitch dashboard body", async () => {
    render(<MonitorPage />);

    expect(await screen.findByRole("heading", { name: /Giám sát rủi ro/i })).toBeTruthy();
    expect(await screen.findByText("128.500.000 ₫")).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Risk Trend Timeline (24h)" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: /Top 5 tài khoản rủi ro cao/i })).toBeTruthy();
    expect(screen.getAllByText("ACC-001234")).toHaveLength(2);
    expect(screen.getByText("Realtime Alert Stream")).toBeTruthy();
  });

  it("changes the time range and shows an empty state for long ranges", async () => {
    render(<MonitorPage />);

    fireEvent.click(screen.getByRole("button", { name: "7 ngày" }));

    expect(await screen.findByText("214")).toBeTruthy();
    expect(await screen.findByText("Chưa có dữ liệu trong khoảng thời gian này.")).toBeTruthy();
    expect(await screen.findByText("Không có alert realtime trong khoảng thời gian này.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "7 ngày" }).getAttribute("aria-pressed")).toBe("true");
  });

  it("renders the Chart.js risk trend canvas", async () => {
    render(<MonitorPage />);

    expect(await screen.findByTestId("chart-canvas")).toBeTruthy();
    expect(screen.getByTestId("risk-trend-chart")).toBeTruthy();
  });

  it("refreshes and toggles the realtime stream", async () => {
    vi.useFakeTimers();
    render(<MonitorPage />);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(180);
    });

    fireEvent.click(screen.getByRole("button", { name: "Làm mới" }));
    expect(screen.getByRole("button", { name: "Đang cập nhật..." })).toBeTruthy();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(180);
    });

    expect(screen.getByRole("button", { name: "Làm mới" })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Tạm dừng luồng" }));
    expect(screen.getByRole("button", { name: "Tiếp tục luồng" })).toBeTruthy();
  });

  it("shows an error and retries the mock request", async () => {
    setMockMonitorFailure(true);
    render(<MonitorPage />);

    expect(await screen.findByRole("alert")).toBeTruthy();
    setMockMonitorFailure(false);
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));

    expect(await screen.findByText("128.500.000 ₫")).toBeTruthy();
  });
});
