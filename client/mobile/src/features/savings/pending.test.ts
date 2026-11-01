import { beforeEach, expect, it } from "vitest";
import { clearPending, loadPending, savePending } from "./pending";

beforeEach(() => sessionStorage.clear());

it("retains an exact uncertain opening command across remounts", () => {
  const command = { kind: "open" as const, quoteId: "quote-1", sourceAccountId: "a1", key: "uuid-key" };
  savePending("user-1", command);
  expect(loadPending("user-1")).toEqual(command);
  expect(loadPending("user-2")).toBeNull();
  clearPending("user-1");
  expect(loadPending("user-1")).toBeNull();
});

it("handles malformed stored commands without executing them", () => {
  sessionStorage.setItem("anomaly:savings:pending:user-1", "{invalid");
  expect(loadPending("user-1")).toBeNull();
});
