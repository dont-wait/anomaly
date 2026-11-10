import { getTransactionReport } from "./transactionsReportApi";
import { reportFixture } from "../testFixtures";

vi.mock("@/features/monitor/api/monitorApi", () => ({
  authHeaders: () => ({ Authorization: "Bearer admin" }),
}));

const fetchMock = vi.fn<typeof fetch>();

// Resolve headers immediately but leave the body open. Real fetch cancels body
// consumption when its own signal aborts, even after the Response was returned.
function pendingBody() {
  fetchMock.mockImplementation(
    async (_input, init) =>
      new Response(
        new ReadableStream({
          start(controller) {
            const fail = () =>
              controller.error(
                new DOMException("Request aborted", "AbortError"),
              );
            if (init?.signal?.aborted) fail();
            else init?.signal?.addEventListener("abort", fail, { once: true });
          },
        }),
      ),
  );
}

describe("transaction report API", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it("uses the single endpoint without version negotiation", async () => {
    const data = reportFixture();
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ status: 200, data })),
    );
    expect(await getTransactionReport(data.date)).toEqual(data);
    const [url, options] = fetchMock.mock.calls[0];
    expect(String(url)).toContain(
      "/api/admin/reports/transactions?date=2026-11-08",
    );
    expect(String(url)).not.toContain("version=");
    expect(options?.headers).toEqual({ Authorization: "Bearer admin" });
  });

  it.each([
    { summary: { transactionCount: 2 } },
    { data: { summary: { transactionCount: 2 } } },
    { data: reportFixture("2026-11-07") },
  ])(
    "rejects obsolete or mismatched response instead of displaying zeros",
    async (body) => {
      fetchMock.mockResolvedValue(new Response(JSON.stringify(body)));
      await expect(getTransactionReport("2026-11-08")).rejects.toThrow(
        /cấu trúc hoặc ngày/,
      );
    },
  );

  it("does not turn an HTTP error into an empty report", async () => {
    fetchMock.mockResolvedValue(new Response("error", { status: 500 }));
    await expect(getTransactionReport("2026-11-08")).rejects.toThrow(
      /Không thể tải/,
    );
  });

  it("rejects an invalid date before sending a request", async () => {
    await expect(getTransactionReport("2026-02-30")).rejects.toThrow(
      /ngày báo cáo hợp lệ/,
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it.each(["null", "<html>not json</html>"])(
    "rejects malformed response %s with a readable error",
    async (body) => {
      fetchMock.mockResolvedValue(new Response(body));
      await expect(getTransactionReport("2026-11-08")).rejects.toThrow(
        /cấu trúc hoặc ngày/,
      );
    },
  );

  it("preserves malformed JSON's original cause alongside the readable error", async () => {
    fetchMock.mockResolvedValue(new Response("not json"));
    await expect(getTransactionReport("2026-11-08")).rejects.toMatchObject({
      message: "Dữ liệu báo cáo không đúng cấu trúc hoặc ngày được chọn.",
      cause: expect.any(SyntaxError),
    });
  });

  it("cancels a stalled JSON body when the caller aborts after receiving headers", async () => {
    pendingBody();
    const controller = new AbortController();
    const remove = vi.spyOn(controller.signal, "removeEventListener");
    const request = getTransactionReport("2026-11-08", controller.signal);
    const assertion = expect(request).rejects.toMatchObject({
      name: "AbortError",
    });
    await Promise.resolve();
    controller.abort();
    await assertion;
    expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(true);
    expect(remove).toHaveBeenCalledWith("abort", expect.any(Function));
  });

  it("times out the entire request including JSON body consumption", async () => {
    vi.useFakeTimers();
    pendingBody();
    const request = getTransactionReport("2026-11-08");
    const assertion = expect(request).rejects.toThrow(/quá thời gian chờ/);
    await vi.advanceTimersByTimeAsync(12000);
    await assertion;
    expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(true);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("cleans up its deadline after success", async () => {
    vi.useFakeTimers();
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ data: reportFixture() })),
    );
    await getTransactionReport("2026-11-08");
    await vi.advanceTimersByTimeAsync(12000);
    expect(fetchMock.mock.calls[0][1]?.signal?.aborted).toBe(false);
    expect(vi.getTimerCount()).toBe(0);
  });
});
