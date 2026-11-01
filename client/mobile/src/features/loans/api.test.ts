import { afterEach, expect, it, vi } from "vitest";
import {
  acceptLoanOffer,
  confirmLoanContract,
  confirmRepayment,
  createLoan,
  getLoanContract,
  listLoanPackages,
  listLoanPayments,
  preQuoteLoan,
  quoteRepayment,
  uploadLoanDocument,
} from "./api";

const fetchMock = vi.fn<typeof fetch>();
vi.stubGlobal("fetch", fetchMock);
afterEach(() => {
  fetchMock.mockReset();
});
const ok = (body: unknown) => {
  fetchMock.mockResolvedValueOnce(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
};

it("uses exact product and quote paths with direct JSON responses", async () => {
  ok({
    items: [
      { packageCode: "PERSONAL_UNSECURED_SALARY", minAmount: 10_000_000 },
    ],
  });
  const packages = await listLoanPackages("jwt");
  expect(packages.items[0].packageCode).toBe("PERSONAL_UNSECURED_SALARY");
  expect(fetchMock.mock.calls[0][0]).toContain("/api/loans/packages");
  expect(
    (fetchMock.mock.calls[0][1]?.headers as Record<string, string>)
      .Authorization,
  ).toBe("Bearer jwt");
  ok({ estimatedMonthlyPayment: 912345, annualRateBps: 1500 });
  const quote = await preQuoteLoan("jwt", {
    loanPackageId: "PERSONAL_UNSECURED_SALARY",
    requestedAmount: 10_000_000,
    termMonths: 12,
    monthlyIncome: 20_000_000,
    regionCode: "KV01",
    existingMonthlyDebt: 0,
  });
  expect(quote.estimatedMonthlyPayment).toBe(912345);
  expect(fetchMock.mock.calls[1][0]).toContain("/api/loans/quotes");
  expect(
    JSON.parse(String(fetchMock.mock.calls[1][1]?.body)).loanPackageId,
  ).toBe("PERSONAL_UNSECURED_SALARY");
});

it("sends idempotency UUID for create and repayment confirm", async () => {
  ok({ id: "loan-app-1", status: "draft" });
  await createLoan(
    "jwt",
    {
      loanPackageId: "PERSONAL_UNSECURED_SALARY",
      requestedAmount: 10_000_000,
      termMonths: 12,
      disbursementAccountId: "account-id",
      declared: {
        monthlyIncome: 20_000_000,
        existingMonthlyDebt: 0,
        regionCode: "KV01",
        purpose: "Mua xe",
      },
    },
    "create-uuid",
  );
  expect(
    (fetchMock.mock.calls[0][1]?.headers as Record<string, string>)[
      "Idempotency-Key"
    ],
  ).toBe("create-uuid");
  ok({ paymentId: "payment-id", status: "success" });
  await confirmRepayment(
    "jwt",
    "loan-id",
    "quote-id",
    "account-id",
    "repay-uuid",
  );
  const args = fetchMock.mock.calls[1];
  expect(args[0]).toContain("/api/loans/loan-id/repayments");
  expect((args[1]?.headers as Record<string, string>)["Idempotency-Key"]).toBe(
    "repay-uuid",
  );
  expect(JSON.parse(String(args[1]?.body))).toEqual({
    quoteId: "quote-id",
    sourceAccountId: "account-id",
  });
});

it("uploads multipart type+file without setting JSON Content-Type", async () => {
  ok({ documentId: "doc-id", type: "IDENTITY_FRONT" });
  await uploadLoanDocument(
    "jwt",
    "application-id",
    "IDENTITY_FRONT",
    new File(["%PDF-test"], "doc.pdf", { type: "application/pdf" }),
  );
  const options = fetchMock.mock.calls[0][1];
  expect(options?.body).toBeInstanceOf(FormData);
  expect((options?.body as FormData).get("type")).toBe("IDENTITY_FRONT");
  expect(
    (options?.headers as Record<string, string>)["Content-Type"],
  ).toBeUndefined();
});

it("uses contract read and repayment/offer paths from the Swagger Word", async () => {
  ok({ loanId: "loan-id", outstandingPrincipal: 9000000 });
  await getLoanContract("jwt", "loan-id");
  expect(fetchMock.mock.calls[0][0]).toContain(
    "/api/loans/contracts/detail/loan-id",
  );
  ok({ quoteId: "repay-quote" });
  await quoteRepayment("jwt", "loan-id", 1, "account-id");
  expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({
    installmentNo: 1,
    sourceAccountId: "account-id",
  });
  ok({ items: [], nextCursor: null });
  await listLoanPayments("jwt", "loan-id", "3");
  expect(fetchMock.mock.calls[2][0]).toContain(
    "/api/loans/loan-id/payments?limit=20&cursor=3",
  );
  ok({ status: "accepted" });
  await acceptLoanOffer("jwt", "application-id");
  expect(fetchMock.mock.calls[3][0]).toContain(
    "/api/loans/application-id/offer/accept",
  );
  ok({ status: "confirmed" });
  await confirmLoanContract("jwt", "application-id", "123456");
  expect(JSON.parse(String(fetchMock.mock.calls[4][1]?.body))).toEqual({
    otp: "123456",
    acceptTerms: true,
  });
});
