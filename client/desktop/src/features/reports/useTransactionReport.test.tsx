import { StrictMode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import {
  getTransactionReport,
  type TransactionReport,
} from "./api/transactionsReportApi";
import { reportFixture } from "./testFixtures";
import { useTransactionReport } from "./useTransactionReport";

vi.mock("./api/transactionsReportApi", () => ({
  getTransactionReport: vi.fn(),
}));

function deferred() {
  let resolve!: (report: TransactionReport) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<TransactionReport>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

async function tick(ms = 0) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

describe("report request lifecycle", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(getTransactionReport).mockReset();
    vi.mocked(getTransactionReport).mockImplementation(async (date) =>
      reportFixture(date),
    );
  });
  afterEach(() => {
    cleanup();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("starts one request in StrictMode and cancels it and polling on unmount", async () => {
    const request = deferred();
    vi.mocked(getTransactionReport).mockReturnValue(request.promise);
    const { result, unmount } = renderHook(
      () => useTransactionReport("2026-11-08"),
      { wrapper: StrictMode },
    );
    expect(result.current.pending).toBe(true);
    expect(result.current.canExport).toBe(false);
    await tick();
    expect(getTransactionReport).toHaveBeenCalledTimes(1);
    const signal = vi.mocked(getTransactionReport).mock.calls[0][1]!;
    unmount();
    expect(signal.aborted).toBe(true);
    await tick(45000);
    expect(getTransactionReport).toHaveBeenCalledTimes(1);
    await act(async () => {
      request.resolve(reportFixture());
    });
  });

  it("does not cancel slow requests on every background polling tick", async () => {
    const slow = deferred();
    vi.mocked(getTransactionReport).mockReturnValueOnce(slow.promise);
    const { result } = renderHook(() => useTransactionReport("2026-11-08"));
    await tick();
    const signal = vi.mocked(getTransactionReport).mock.calls[0][1]!;
    await tick(30000);
    expect(getTransactionReport).toHaveBeenCalledTimes(1);
    expect(signal.aborted).toBe(false);
    expect(result.current.pending).toBe(true);
    await act(async () => {
      slow.resolve(reportFixture());
    });
    expect(result.current.report?.date).toBe("2026-11-08");
    expect(result.current.canExport).toBe(true);
    await tick(15000);
    expect(getTransactionReport).toHaveBeenCalledTimes(2);
  });

  it("removes the old day immediately and ignores its late result after switching days", async () => {
    const old = deferred();
    vi.mocked(getTransactionReport).mockReturnValueOnce(old.promise);
    const { result, rerender } = renderHook(
      ({ date }) => useTransactionReport(date),
      { initialProps: { date: "2026-11-08" } },
    );
    await tick();
    const signal = vi.mocked(getTransactionReport).mock.calls[0][1]!;
    rerender({ date: "2026-11-07" });
    expect(result.current.report).toBeNull();
    expect(result.current.pending).toBe(true);
    expect(result.current.canExport).toBe(false);
    await tick();
    expect(signal.aborted).toBe(true);
    expect(result.current.report?.date).toBe("2026-11-07");
    await act(async () => {
      old.resolve(reportFixture("2026-11-08"));
    });
    expect(result.current.report?.date).toBe("2026-11-07");
  });

  it("ignores a late failure from a superseded explicit refresh", async () => {
    const old = deferred();
    const latest = deferred();
    vi.mocked(getTransactionReport)
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(latest.promise);
    const { result } = renderHook(() => useTransactionReport("2026-11-08"));
    await tick();
    act(() => {
      void result.current.refresh();
    });
    await act(async () => {
      latest.resolve(reportFixture());
    });
    expect(result.current.canExport).toBe(true);
    await act(async () => {
      old.reject(new Error("obsolete failure"));
    });
    expect(result.current.error).toBeNull();
    expect(result.current.pending).toBe(false);
    expect(result.current.canExport).toBe(true);
  });

  it("retains stale data and its asOf through failed refresh and retry, until a successful response", async () => {
    const { result } = renderHook(() => useTransactionReport("2026-11-08"));
    await tick();
    const asOf = result.current.report!.asOf;
    vi.mocked(getTransactionReport).mockRejectedValueOnce(
      new Error("network failed"),
    );
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.report?.asOf).toBe(asOf);
    expect(result.current.stale).toBe(true);
    expect(result.current.error).toBe("network failed");
    expect(result.current.canExport).toBe(false);
    const retry = deferred();
    vi.mocked(getTransactionReport).mockReturnValueOnce(retry.promise);
    act(() => {
      void result.current.refresh();
    });
    expect(result.current.stale).toBe(true);
    expect(result.current.pending).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.canExport).toBe(false);
    const updated = reportFixture();
    updated.asOf = "2026-11-08T13:00:00Z";
    await act(async () => {
      retry.resolve(updated);
    });
    expect(result.current.report?.asOf).toBe(updated.asOf);
    expect(result.current.stale).toBe(false);
    expect(result.current.canExport).toBe(true);
  });

  it("does not invent a zero report on first load failure and allows retry", async () => {
    vi.mocked(getTransactionReport).mockRejectedValueOnce(
      new Error("first load failed"),
    );
    const { result } = renderHook(() => useTransactionReport("2026-11-08"));
    await tick();
    expect(result.current.report).toBeNull();
    expect(result.current.error).toBe("first load failed");
    expect(result.current.stale).toBe(false);
    expect(result.current.canExport).toBe(false);
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.report).not.toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("does not fetch or poll invalid dates and discards the previously selected day", async () => {
    const { result, rerender } = renderHook(
      ({ date }) => useTransactionReport(date),
      { initialProps: { date: "2026-11-08" } },
    );
    await tick();
    rerender({ date: "" });
    expect(result.current.report).toBeNull();
    await tick(45000);
    expect(getTransactionReport).toHaveBeenCalledTimes(1);
    expect(result.current.error).toContain("ngày báo cáo hợp lệ");
    expect(result.current.pending).toBe(false);
    expect(result.current.canExport).toBe(false);
  });
});
