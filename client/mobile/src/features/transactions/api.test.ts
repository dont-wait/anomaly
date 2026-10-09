import { afterEach, expect, it, vi } from "vitest";
import { getTransactionSummary, listTransactions } from "./api";

afterEach(() => vi.unstubAllGlobals());

it("requests server-side filters and maps transaction dates", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
        items: [
          {
            id: "tx-1",
            reference: "FT1",
            direction: "out",
            status: "success",
            kind: "transfer",
            amount: 100_000,
            fee: 0,
            note: "Tien an",
            counterparty: { name: "Nguyen A", bankCode: "ANOMALY" },
            createdAt: "2026-10-09T01:02:03Z",
          },
        ],
        nextCursor: "opaque-cursor",
      }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);

  const page = await listTransactions("access-token", {
    direction: "out",
    query: "  tiền ăn  ",
    cursor: "opaque-cursor",
    limit: 10,
  });

  const requestUrl = new URL(String(fetchMock.mock.calls[0][0]));
  expect(requestUrl.pathname).toBe("/api/transactions");
  expect(Object.fromEntries(requestUrl.searchParams)).toEqual({
    direction: "out",
    limit: "10",
    q: "tiền ăn",
    cursor: "opaque-cursor",
  });
  expect(fetchMock.mock.calls[0][1]).toMatchObject({
    headers: { Authorization: "Bearer access-token" },
  });
  expect(page.nextCursor).toBe("opaque-cursor");
  expect(page.items[0].createdAt).toEqual(new Date("2026-10-09T01:02:03Z"));
  expect(page.items[0].counterparty.bank).toBe("ANOMALY");
});

it("requests a complete monthly summary independently of the transaction page", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: async () =>
      JSON.stringify({
        month: "2026-09",
        totalIn: 5_000_000,
        totalOut: 700_000,
      }),
  } as Response);
  vi.stubGlobal("fetch", fetchMock);

  const summary = await getTransactionSummary(
    "access-token",
    new Date("2026-08-31T18:00:00Z"),
  );

  const requestUrl = new URL(String(fetchMock.mock.calls[0][0]));
  expect(requestUrl.pathname).toBe("/api/transactions/summary");
  expect(requestUrl.searchParams.get("month")).toBe("2026-09");
  expect(summary.totalIn).toBe(5_000_000);
});
