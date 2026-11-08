import { fetchWithTimeout } from "@/features/monitor/api/monitorApi";
import { getTransactionReport } from "./transactionsReportApi";
import { reportFixture } from "../testFixtures";

vi.mock("@/features/monitor/api/monitorApi", () => ({
  authHeaders: () => ({ Authorization: "Bearer admin" }),
  fetchWithTimeout: vi.fn(),
}));

describe("transaction report API", () => {
  afterEach(() => vi.clearAllMocks());

  it("uses the single endpoint without version negotiation", async () => {
    const data = reportFixture();
    vi.mocked(fetchWithTimeout).mockResolvedValue(
      new Response(JSON.stringify({ status: 200, data })),
    );
    expect(await getTransactionReport(data.date)).toEqual(data);
    const [url, options] = vi.mocked(fetchWithTimeout).mock.calls[0];
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
      vi.mocked(fetchWithTimeout).mockResolvedValue(
        new Response(JSON.stringify(body)),
      );
      await expect(getTransactionReport("2026-11-08")).rejects.toThrow(
        /cấu trúc hoặc ngày/,
      );
    },
  );

  it("does not turn an HTTP error into an empty report", async () => {
    vi.mocked(fetchWithTimeout).mockResolvedValue(
      new Response("error", { status: 500 }),
    );
    await expect(getTransactionReport("2026-11-08")).rejects.toThrow(
      /Không thể tải/,
    );
  });
});
