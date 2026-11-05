import { render, screen, waitFor } from "@testing-library/react";
import { AdminAlertCountProvider } from "./AdminAlertCountProvider";
import { useAdminAlertCount } from "./adminAlertCountContext";
import {
  consumeRiskStream,
  getMonitorDashboard,
} from "@/features/monitor/api/monitorApi";

vi.mock("@/features/monitor/api/monitorApi", () => ({
  consumeRiskStream: vi.fn(
    async (
      _cursor: string,
      onEvent: (
        event: { kind: string; count?: number },
        cursor: string,
      ) => void,
      _signal: AbortSignal,
      onConnected?: () => void,
    ) => {
      onConnected?.();
      onEvent({ kind: "alert_count", count: 7 }, "cursor-2");
      await new Promise<void>(() => undefined);
    },
  ),
  getMonitorDashboard: vi.fn().mockResolvedValue({
    cursor: "cursor-1",
    snapshot: { unhandledAlertCount: 4 },
  }),
  shouldAdvanceMonitorCursor: vi.fn(() => true),
}));

function CountProbe() {
  return (
    <output aria-label="unhandled alert count">{useAdminAlertCount()}</output>
  );
}

describe("AdminAlertCountProvider", () => {
  afterEach(() => vi.clearAllMocks());

  it("loads the current count and applies realtime updates", async () => {
    render(
      <AdminAlertCountProvider>
        <CountProbe />
      </AdminAlertCountProvider>,
    );

    await waitFor(() => expect(screen.getByText("7")).toBeTruthy());
    expect(getMonitorDashboard).toHaveBeenCalledWith("24h", undefined, "all");
    expect(consumeRiskStream).toHaveBeenCalled();
  });
});
