import { render, screen, waitFor } from "@testing-library/react";
import { AdminAlertCountProvider } from "./AdminAlertCountProvider";
import { useAdminAlertCount } from "./adminAlertCountContext";
import {
  consumeUnhandledAlertCountStream,
  getUnhandledAlertCount,
} from "@/features/monitor/api/monitorApi";

vi.mock("@/features/monitor/api/monitorApi", () => ({
  consumeUnhandledAlertCountStream: vi.fn(
    async (
      onCount: (count: number) => void,
      _signal: AbortSignal,
      onConnected?: () => void,
    ) => {
      onConnected?.();
      onCount(7);
      await new Promise<void>(() => undefined);
    },
  ),
  getUnhandledAlertCount: vi.fn().mockResolvedValue(4),
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
    expect(getUnhandledAlertCount).toHaveBeenCalled();
    expect(consumeUnhandledAlertCountStream).toHaveBeenCalled();
  });
});
