import { expect, it } from "vitest";
import {
  isProtectedRoute,
  parseTransactionId,
  routes,
  transactionDetailRoute,
} from "./routes";

it("builds and parses transaction detail routes", () => {
  const route = transactionDetailRoute("tx 1/2");
  expect(parseTransactionId(route)).toBe("tx 1/2");
  expect(parseTransactionId(routes.transactions)).toBeNull();
  expect(parseTransactionId(`${routes.transactions}/`)).toBeNull();
});

it("marks transaction pages as protected", () => {
  expect(isProtectedRoute(routes.transfer)).toBe(true);
  expect(isProtectedRoute(routes.transactions)).toBe(true);
  expect(isProtectedRoute(transactionDetailRoute("tx_1001"))).toBe(true);
  expect(isProtectedRoute(routes.login)).toBe(false);
});

it("ignores malformed percent-encoding instead of throwing", () => {
  expect(parseTransactionId(`${routes.transactions}/%E0`)).toBeNull();
  expect(isProtectedRoute(`${routes.transactions}/100%`)).toBe(false);
});
