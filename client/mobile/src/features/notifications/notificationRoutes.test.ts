import { expect, it } from "vitest";
import { loanContract, routes, transactionDetailRoute } from "@/app/routes";
import type { AppNotification } from "./model";
import {
  registerNotificationRoute,
  routeForNotification,
} from "./notificationRoutes";

const notification = (
  type: string,
  data: Record<string, string> = {},
): AppNotification => ({
  id: "n1",
  type,
  category: "TRANSACTION",
  severity: "SUCCESS",
  title: "t",
  body: "b",
  data,
  occurredAt: new Date(),
  createdAt: new Date(),
  read: false,
});

it("routes transfer notifications to the transaction detail", () => {
  expect(
    routeForNotification(
      notification("TRANSFER_RECEIVED", { transferId: "tx 1" }),
    ),
  ).toBe(transactionDetailRoute("tx 1"));
});

it("falls back to the notification center", () => {
  expect(routeForNotification(notification("SYSTEM_TEST"))).toBe(
    routes.notifications,
  );
  expect(routeForNotification(notification("TRANSFER_SENT"))).toBe(
    routes.notifications,
  );
});

it("lets modules register their own route", () => {
  registerNotificationRoute("LOAN_APPROVED", () => routes.loanApplications);
  expect(routeForNotification(notification("LOAN_APPROVED"))).toBe(
    routes.loanApplications,
  );
});

it("opens loan repayment reminders at their own loan contract", () => {
  expect(
    routeForNotification(
      notification("LOAN_REPAYMENT_DUE", { loanId: "loan 123" }),
    ),
  ).toBe(loanContract("loan 123"));
  expect(routeForNotification(notification("LOAN_REPAYMENT_DUE"))).toBe(
    routes.notifications,
  );
});
