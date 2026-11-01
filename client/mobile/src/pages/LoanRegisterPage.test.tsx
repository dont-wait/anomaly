import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
import { createLoan, preQuoteLoan } from "@/features/loans/api";
import LoanRegisterPage from "./LoanRegisterPage";

vi.mock("@/features/loans/api", () => ({
  listLoanPackages: vi.fn(async () => ({
    items: [
      {
        packageCode: "PERSONAL_UNSECURED_SALARY",
        name: "Vay cá nhân",
        minAmount: 10_000_000,
        maxAmount: 200_000_000,
        termMonths: [6, 12, 24, 36],
      },
    ],
  })),
  preQuoteLoan: vi.fn(),
  createLoan: vi.fn(),
}));

const auth: AuthContextValue = {
  status: AUTH_STATUS.AUTHENTICATED,
  user: {
    id: "user-1",
    accountNo: "999991801234321",
    username: "Test User",
    fullName: "Test User",
    email: "test@example.com",
    currency: "VND",
    isVerify: true,
    amount: 1_500_000,
  },
  token: "access-token",
  error: null,
  login: async () => auth.user!,
  logout: () => undefined,
  refreshProfile: async () => auth.user,
  updateProfile: async () => auth.user!,
  changePassword: async () => undefined,
  uploadProfileAvatar: async () => auth.user,
  uploadProfileDocument: async () => auth.user,
  startEmailChange: async () => auth.user,
  confirmEmailChange: async () => auth.user,
};

const renderPage = () =>
  render(
    <AuthContext.Provider value={auth}>
      <LoanRegisterPage />
    </AuthContext.Provider>,
  );

beforeEach(() => {
  localStorage.clear();
  vi.mocked(preQuoteLoan).mockReset();
  vi.mocked(createLoan).mockReset();
});
afterEach(cleanup);

it("lets the user select the advertised 200M maximum", async () => {
  renderPage();
  await amountReady();

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("50.000.000");

  fireEvent.click(screen.getByRole("radio", { name: "200 Tr" }));

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("200.000.000");
});

const next = () =>
  fireEvent.click(screen.getByRole("button", { name: "Tiếp tục" }));
const amountReady = async () => {
  await waitFor(() =>
    expect(
      screen.getByRole<HTMLButtonElement>("button", { name: "Tiếp tục" })
        .disabled,
    ).toBe(false),
  );
};

it("advances choices only on continue or a double tap", async () => {
  renderPage();
  await amountReady();
  expect(screen.getByText("Bước 1/7")).toBeTruthy();
  expect(screen.queryByLabelText("Thu nhập mỗi tháng")).toBeNull();

  next();
  expect(screen.queryByLabelText("Số tiền đề nghị vay")).toBeNull();
  expect(screen.getByText(/chạm 2 lần/)).toBeTruthy();
  const choice = screen.getByRole("radio", { name: "24 tháng" });
  fireEvent.click(choice, { timeStamp: 0 });
  expect(screen.getByText("Bước 2/7")).toBeTruthy();
  expect(choice.getAttribute("aria-checked")).toBe("true");

  next();
  expect(screen.getByText("Bước 3/7")).toBeTruthy();
  const car = screen.getByRole("radio", { name: "Mua xe" });
  fireEvent.click(car);
  fireEvent.click(car);
  expect(screen.getByLabelText("Thu nhập mỗi tháng")).toBeTruthy();
});

it("blocks continuing with an amount outside the package limit", async () => {
  renderPage();
  await amountReady();
  fireEvent.change(screen.getByLabelText("Số tiền đề nghị vay"), {
    target: { value: "1000" },
  });

  expect(
    screen.getByRole<HTMLButtonElement>("button", { name: "Tiếp tục" })
      .disabled,
  ).toBe(true);
});

it("keeps entered data when going back", async () => {
  renderPage();
  await amountReady();
  fireEvent.click(screen.getByRole("radio", { name: "100 Tr" }));
  next();
  fireEvent.click(screen.getByRole("button", { name: "Quay lại" }));

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("100.000.000");
});

it("restores the draft on the same step after remounting", async () => {
  const first = renderPage();
  await amountReady();
  fireEvent.click(screen.getByRole("radio", { name: "100 Tr" }));
  next();
  first.unmount();

  renderPage();

  expect(screen.getByRole("radiogroup", { name: "Kỳ hạn vay" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Quay lại" }));
  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("100.000.000");
});

it("survives a corrupted draft", () => {
  localStorage.setItem(
    "anomaly:loan-draft:user-1",
    JSON.stringify({ amount: "x", step: "nope", region: 1, income: -5 }),
  );
  renderPage();

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("50.000.000");
});

it("requires a detail when the custom purpose is selected", async () => {
  localStorage.setItem(
    "anomaly:loan-draft:user-1",
    JSON.stringify({ step: "purpose" }),
  );
  renderPage();

  fireEvent.click(
    screen.getByRole("radio", { name: "Khác (Cung cấp chi tiết)" }),
  );
  const button = screen.getByRole<HTMLButtonElement>("button", {
    name: "Tiếp tục",
  });
  expect(button.disabled).toBe(true);

  fireEvent.change(
    screen.getByPlaceholderText("Mô tả mục đích sử dụng khoản vay..."),
    { target: { value: "Vay mua thiết bị văn phòng" } },
  );
  expect(button.disabled).toBe(false);
});

const quoteResult = {
  loanPackageId: "PERSONAL_UNSECURED_SALARY",
  requestedAmount: 50_000_000,
  termMonths: 12,
  annualRateBps: 1200,
  estimatedMonthlyPayment: 4_500_000,
  totalInterest: 4_000_000,
  totalRepayment: 54_000_000,
  affordability: {
    withinIncomeLimit: true,
    dtiStatus: "pass",
    dtiBps: 2000,
    incomeLimit: 100_000_000,
  },
  provisionalSchedule: [],
  official: false,
};

it("quotes on reaching review and creates the same payload as before", async () => {
  vi.mocked(preQuoteLoan).mockResolvedValue(quoteResult);
  vi.mocked(createLoan).mockResolvedValue({ id: "app-1" } as never);
  localStorage.setItem(
    "anomaly:loan-draft:user-1",
    JSON.stringify({ step: "region" }),
  );
  renderPage();
  await waitFor(() =>
    expect(
      screen.getByRole<HTMLButtonElement>("radio", { name: "Khu vực khác" }),
    ).toBeTruthy(),
  );
  await amountReady();

  fireEvent.click(screen.getByRole("radio", { name: "Khu vực khác" }));
  next();

  expect(await screen.findByText("4.500.000 đ")).toBeTruthy();
  expect(vi.mocked(preQuoteLoan).mock.calls[0][1]).toMatchObject({
    regionCode: "KV03",
    requestedAmount: 50_000_000,
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Tiếp tục tải chứng từ" }),
  );

  await waitFor(() =>
    expect(createLoan).toHaveBeenCalledWith(
      "access-token",
      {
        loanPackageId: "PERSONAL_UNSECURED_SALARY",
        requestedAmount: 50_000_000,
        termMonths: 12,
        disbursementAccountId: "user-1",
      },
      expect.any(String),
    ),
  );
  expect(localStorage.getItem("anomaly:loan-draft:user-1")).toBeNull();
});

it("discards the quote when an earlier answer changes", async () => {
  vi.mocked(preQuoteLoan).mockResolvedValue(quoteResult);
  localStorage.setItem(
    "anomaly:loan-draft:user-1",
    JSON.stringify({ step: "region" }),
  );
  renderPage();
  await amountReady();
  fireEvent.click(screen.getByRole("radio", { name: "Khu vực khác" }));
  next();
  await screen.findByText("4.500.000 đ");

  fireEvent.click(screen.getByRole("button", { name: "Quay lại" }));
  fireEvent.click(screen.getByRole("button", { name: "Quay lại" }));
  fireEvent.change(screen.getByLabelText("Khoản trả nợ hiện tại mỗi tháng"), {
    target: { value: "1000000" },
  });
  next();
  next();

  await waitFor(() => expect(preQuoteLoan).toHaveBeenCalledTimes(2));
  expect(vi.mocked(preQuoteLoan).mock.calls[1][1]).toMatchObject({
    existingMonthlyDebt: 1_000_000,
  });
});

it("stays on review with a retry when the quote fails", async () => {
  vi.mocked(preQuoteLoan).mockRejectedValue(new Error("Lỗi báo giá"));
  localStorage.setItem(
    "anomaly:loan-draft:user-1",
    JSON.stringify({ step: "region" }),
  );
  renderPage();
  await amountReady();
  fireEvent.click(screen.getByRole("radio", { name: "Khu vực khác" }));
  next();

  expect((await screen.findByRole("alert")).textContent).toContain(
    "Lỗi báo giá",
  );
  expect(
    screen.getByRole("button", { name: "Xem báo giá dự kiến" }),
  ).toBeTruthy();
  expect(
    screen.getByRole<HTMLButtonElement>("button", {
      name: "Tiếp tục tải chứng từ",
    }).disabled,
  ).toBe(true);
});

it("formats typed amounts and keeps only digits", () => {
  renderPage();
  const input = screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay");

  fireEvent.change(input, { target: { value: "75000000abc" } });

  expect(input.value).toBe("75.000.000");
  expect(input.getAttribute("inputmode")).toBe("numeric");
  expect(screen.getByText(/75 triệu đồng/)).toBeTruthy();

  fireEvent.change(input, { target: { value: "" } });
  expect(input.value).toBe("");
});

it("suggests amounts while typing and hides them after picking one", () => {
  renderPage();
  const input = screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay");
  expect(
    screen.queryByRole("group", { name: /Gợi ý số tiền đề nghị vay/ }),
  ).toBeNull();

  fireEvent.change(input, { target: { value: "3" } });
  fireEvent.click(screen.getByRole("button", { name: "300.000" }));

  expect(input.value).toBe("300.000");
  expect(screen.queryByRole("button", { name: "3.000" })).toBeNull();
});
