import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/shared/lib/http";
import {
  createSavingsQuote, createSavingsWithdrawalQuote, describeSavingsError,
  getSavingsContract, getSavingsHistory, getSavingsReceipt, getSavingsTerms, listSavingsContracts,
  listSavingsPackages, mayBeCommitted, openSavingsContract, withdrawSavings,
} from "./api";

const response = (status: number, body: unknown) => ({
  ok: status >= 200 && status < 300, status,
  text: async () => JSON.stringify(body),
}) as Response;

afterEach(() => vi.unstubAllGlobals());

describe("live Savings API contract", () => {
  it("uses JWT and direct JSON responses (no auth data envelope)", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => response(200, { items: [{ packageId: "savings_standard" }] }));
    vi.stubGlobal("fetch", fetchMock);
    const catalog = await listSavingsPackages("user-token");
    expect(catalog.items[0].packageId).toBe("savings_standard");
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toMatch(/\/api\/savings\/packages$/);
    expect((init.headers as Record<string,string>).Authorization).toBe("Bearer user-token");
  });

  it("supplies server-side status=all and a cursor, encodes detail IDs", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => response(200, { items: [], nextCursor: null }));
    vi.stubGlobal("fetch", fetchMock);
    await listSavingsContracts("jwt", "opaque+/=");
    expect(fetchMock.mock.calls[0][0]).toContain("status=all&limit=50&cursor=opaque%2B%2F%3D");
    await getSavingsContract("jwt", "sav/id");
    expect(fetchMock.mock.calls[1][0]).toContain("/contracts/sav%2Fid");
  });

  it("keeps the caller-provided UUID for opening and withdrawal commands", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => response(201, { savingsId: "sav-1" }));
    vi.stubGlobal("fetch", fetchMock);
    const key = "e6b63de1-52eb-49fd-aa33-8fef301cf866";
    await createSavingsQuote("jwt", { packageId: "savings_standard", sourceAccountId: "a1", amount: 1_000_000, termMonths: 3 });
    await openSavingsContract("jwt", { quoteId: "q1", sourceAccountId: "a1", confirm: true }, key);
    await createSavingsWithdrawalQuote("jwt", "sav-1");
    await withdrawSavings("jwt", "sav-1", "q2", key);
    await getSavingsReceipt("jwt", "sav-1");
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({ quoteId: "q1", sourceAccountId: "a1", confirm: true });
    expect((fetchMock.mock.calls[1][1]?.headers as Record<string,string>)["Idempotency-Key"]).toBe(key);
    expect(JSON.parse(String(fetchMock.mock.calls[2][1]?.body))).toEqual({ withdrawalType: "full" });
    expect(JSON.parse(String(fetchMock.mock.calls[3][1]?.body))).toEqual({ quoteId: "q2", confirm: true });
    expect((fetchMock.mock.calls[3][1]?.headers as Record<string,string>)["Idempotency-Key"]).toBe(key);
    expect(fetchMock.mock.calls[4][0]).toMatch(/\/settlement$/);
  });

  it("never treats 503 or network failures as confirmed writes", () => {
    expect(mayBeCommitted(new ApiError(503, "outcome unknown"))).toBe(true);
    expect(mayBeCommitted(new ApiError(0, "offline"))).toBe(true);
    expect(mayBeCommitted(new ApiError(410, "expired"))).toBe(false);
    expect(describeSavingsError(new ApiError(503, "unavailable", { code: "SAVINGS_WRITES_DISABLED" }))).toContain("SAVINGS_WRITES_DISABLED");
  });
});

describe("Savings disclosures and history", () => {
  it("loads versioned product terms with JWT and account-scoped history", async () => {
    const fetchMock = vi.fn(async (_url: string, _init?: RequestInit) => response(200, { items: [], version: "savings-standard-v1", sections: [] }));
    vi.stubGlobal("fetch", fetchMock);
    await getSavingsTerms("jwt");
    await getSavingsHistory("jwt", "sav/a");
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/api\/savings\/terms$/);
    expect(fetchMock.mock.calls[1][0]).toContain("/contracts/sav%2Fa/history");
    expect((fetchMock.mock.calls[0][1]?.headers as Record<string, string>).Authorization).toBe("Bearer jwt");
    expect((fetchMock.mock.calls[1][1]?.headers as Record<string, string>).Authorization).toBe("Bearer jwt");
  });
});
