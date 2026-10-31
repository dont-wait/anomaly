import { beforeEach, expect, it } from "vitest";
import { clearIdempotencyKey, getOrCreateIdempotencyKey } from "./idempotency";

beforeEach(() => {
  sessionStorage.clear();
});

it("reuses the same key for the same logical request", () => {
  const scope = "create-loan:user-1";
  const fingerprint = JSON.stringify({ amount: 10_000_000 });

  const first = getOrCreateIdempotencyKey(scope, fingerprint);
  const second = getOrCreateIdempotencyKey(scope, fingerprint);

  expect(second).toBe(first);
});

it("separates requests by scope and payload", () => {
  const a = getOrCreateIdempotencyKey("create-loan:user-1", "payload-a");
  const b = getOrCreateIdempotencyKey("create-loan:user-2", "payload-a");
  const c = getOrCreateIdempotencyKey("create-loan:user-1", "payload-b");

  expect(new Set([a, b, c]).size).toBe(3);
});

it("clears the key after the backend confirms the write", () => {
  const scope = "repay:user-1:loan-1";
  const fingerprint = "quote-1:account-1";
  const first = getOrCreateIdempotencyKey(scope, fingerprint);

  clearIdempotencyKey(scope, fingerprint);
  const afterClear = getOrCreateIdempotencyKey(scope, fingerprint);

  expect(afterClear).not.toBe(first);
});
