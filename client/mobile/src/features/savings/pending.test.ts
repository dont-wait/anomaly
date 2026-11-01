import { beforeEach, expect, it, vi } from "vitest";
import { clearPending, loadPending, savePending } from "./pending";

beforeEach(() => { localStorage.clear(); sessionStorage.clear(); });

it("retains an exact uncertain opening command across remounts", () => {
  const command = { kind: "open" as const, quoteId: "quote-1", sourceAccountId: "a1", acceptedTermsVersion: "terms-v1", key: "uuid-key" };
  savePending("user-1", command);
  expect(loadPending("user-1")).toEqual(command);
  expect(loadPending("user-2")).toBeNull();
  clearPending("user-1");
  expect(loadPending("user-1")).toBeNull();
});

it("handles malformed stored commands without executing them", () => {
  localStorage.setItem("anomaly:savings:pending:user-1", "{invalid");
  expect(() => loadPending("user-1")).toThrow();
});

it("blocks financial commands when durable storage cannot be written", () => {
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("full"); });
  expect(() => savePending("user-1", {kind: "withdraw",key: "key",quoteId:"q",savingsId:"sav"})).toThrow();
  spy.mockRestore();
});

it("refuses to forget a legacy session-scoped pending command", () => {
  sessionStorage.setItem("anomaly:savings:pending:user-1", JSON.stringify({kind:"open", key:"old", quoteId:"q", sourceAccountId:"a1"}));
  expect(() => loadPending("user-1")).toThrow();
});
