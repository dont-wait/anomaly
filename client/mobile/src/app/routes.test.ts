import { expect, it } from "vitest";
import {
  isProtectedRoute,
  parseTransactionId,
  parseLoanApplicationId,
  parseLoanContractId,
  loanApplication,
  loanContract,
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
  expect(isProtectedRoute(routes.loanRegister)).toBe(true);
  expect(isProtectedRoute(routes.savings)).toBe(true);
  expect(isProtectedRoute(transactionDetailRoute("tx_1001"))).toBe(true);
  expect(isProtectedRoute(routes.login)).toBe(false);
});

it("ignores malformed percent-encoding instead of throwing", () => {
  expect(parseTransactionId(`${routes.transactions}/%E0`)).toBeNull();
  expect(isProtectedRoute(`${routes.transactions}/100%`)).toBe(false);
});

it("supports protected loan application and contract detail routes", () => {
  const application = loanApplication("app 1/2");
  const contract = loanContract("loan-1");
  expect(parseLoanApplicationId(application)).toBe("app 1/2");
  expect(parseLoanContractId(contract)).toBe("loan-1");
  expect(isProtectedRoute(application)).toBe(true);
  expect(isProtectedRoute(contract)).toBe(true);
  expect(isProtectedRoute(routes.loanApplications)).toBe(true);
  expect(isProtectedRoute(routes.loanContracts)).toBe(true);
  expect(parseLoanContractId(`${routes.loanContracts}/%E0`)).toBeNull();
});
