import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
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

beforeEach(() => localStorage.clear());
afterEach(cleanup);

it("lets the user select the advertised 200M maximum", () => {
  renderPage();

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("50.000.000");

  fireEvent.click(screen.getByRole("radio", { name: "200 Tr" }));

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("200.000.000");
});

it("restores a saved draft after remounting", () => {
  const first = renderPage();

  fireEvent.click(screen.getByRole("radio", { name: "100 Tr" }));
  fireEvent.click(screen.getByRole("button", { name: "Lưu tạm trên máy" }));

  expect(screen.getByText(/Đã lưu tạm/)).toBeTruthy();
  first.unmount();

  renderPage();

  expect(
    screen.getByLabelText<HTMLInputElement>("Số tiền đề nghị vay").value,
  ).toBe("100.000.000");
});

it("requires a detail when the custom purpose is selected", () => {
  renderPage();

  fireEvent.click(screen.getByRole("button", { name: /Tiêu dùng cá nhân/ }));
  fireEvent.click(
    screen.getByRole("option", { name: "Khác (Cung cấp chi tiết)" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Lưu tạm trên máy" }));

  expect(screen.getByRole("alert").textContent).toContain(
    "Vui lòng nhập mục đích vay cụ thể",
  );

  fireEvent.change(
    screen.getByPlaceholderText("Mô tả mục đích sử dụng khoản vay..."),
    {
      target: { value: "Vay mua thiết bị văn phòng" },
    },
  );
  fireEvent.click(screen.getByRole("button", { name: "Lưu tạm trên máy" }));

  expect(screen.getByText(/Đã lưu tạm/)).toBeTruthy();
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
