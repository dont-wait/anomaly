import { Toaster } from "@/shared/notifications/Toaster";
import { toast } from "@/shared/notifications/toast";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
import { AppRouter } from "./AppRouter";
import { loanApplication, routes } from "./routes";
import { getLoan, listLoans, type LoanApplication } from "@/features/loans/api";

vi.mock("@/features/loans/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/loans/api")>()),
  listLoans: vi.fn(),
  getLoan: vi.fn(),
}));

const auth: AuthContextValue = {
  status: AUTH_STATUS.UNAUTHENTICATED,
  user: null,
  token: null,
  error: null,
  login: vi.fn(async () => {
    auth.status = AUTH_STATUS.AUTHENTICATED;
    auth.user = {
      id: "account-id",
      accountNo: "ACC-001",
      username: "Test User",
      email: "test@example.com",
      currency: "VND",
      idCardFrontUrl: "",
      idCardBackUrl: "",
      liveVideoUrl: "",
      isVerify: false,
      amount: 0,
    };
    return auth.user;
  }),
  logout: vi.fn(),
  refreshProfile: vi.fn(),
  updateProfile: vi.fn(),
  changePassword: vi.fn(),
  uploadProfileAvatar: vi.fn(),
  uploadProfileDocument: vi.fn(),
};
function renderRouter() {
  return render(
    <AuthContext.Provider value={auth}>
      <AppRouter />
      <Toaster />
    </AuthContext.Provider>,
  );
}
beforeEach(() => {
  auth.status = AUTH_STATUS.UNAUTHENTICATED;
  auth.user = null;
  auth.token = null;
  window.history.replaceState(null, "", "/");
});
afterEach(() => {
  cleanup();
  toast.dismiss();
  window.history.replaceState(null, "", "/");
});

it("opens registration from the new login page and returns to the real login form", async () => {
  renderRouter();
  const link = screen.getByRole("link", { name: /Mở tài khoản ngay/ });
  expect(link.getAttribute("href")).toBe(routes.register);
  fireEvent.click(link);
  await screen.findByRole("heading", { name: "Mở tài khoản AnomalyBank" });
  expect(screen.getByRole("textbox", { name: /Địa chỉ email/ })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
  await screen.findByRole("link", { name: /Mở tài khoản ngay/ });
  expect(window.location.hash).toBe(routes.login);
  expect(screen.getByLabelText("Số Căn cước Công dân")).toBeTruthy();
});

it("supports direct registration links and browser Back", async () => {
  window.history.replaceState(null, "", routes.login);
  renderRouter();
  fireEvent.click(screen.getByRole("link", { name: /Mở tài khoản ngay/ }));
  await screen.findByRole("heading", { name: "Mở tài khoản AnomalyBank" });
  window.history.back();
  await waitFor(() => expect(window.location.hash).toBe(routes.login));
  await screen.findByRole("link", { name: /Mở tài khoản ngay/ });
  cleanup();
  toast.dismiss();
  window.history.replaceState(null, "", routes.register);
  renderRouter();
  expect(
    screen.getByRole("heading", { name: "Mở tài khoản AnomalyBank" }),
  ).toBeTruthy();
});

it("keeps the CCCD login submission when returning from registration", async () => {
  renderRouter();
  fireEvent.click(screen.getByRole("link", { name: /Mở tài khoản ngay/ }));
  await screen.findByRole("heading", { name: "Mở tài khoản AnomalyBank" });
  fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
  await screen.findByRole("link", { name: /Mở tài khoản ngay/ });
  fireEvent.change(screen.getByLabelText("Số Căn cước Công dân"), {
    target: { value: "012345678901" },
  });
  fireEvent.change(screen.getByLabelText("Mật khẩu"), {
    target: { value: "password123" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
  await waitFor(() =>
    expect(auth.login).toHaveBeenCalledWith({
      cccdNumber: "012345678901",
      password: "password123",
    }),
  );
  await waitFor(() => expect(window.location.hash).toBe(routes.dashboard));
  await screen.findByRole("heading", { name: "Giao dịch gần đây" });
});

it("stays on login when authentication fails", async () => {
  vi.mocked(auth.login).mockRejectedValueOnce(new Error("Đăng nhập thất bại"));
  window.history.replaceState(null, "", routes.login);
  renderRouter();
  fireEvent.change(screen.getByLabelText("Số Căn cước Công dân"), {
    target: { value: "012345678901" },
  });
  fireEvent.change(screen.getByLabelText("Mật khẩu"), {
    target: { value: "password123" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));
  const alert = await screen.findByRole("alert");
  expect(alert.closest("form")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Đóng thông báo" }));
  expect(screen.queryByRole("alert")).toBeNull();
  expect(window.location.hash).toBe(routes.login);
  expect(screen.getByLabelText("Số Căn cước Công dân")).toBeTruthy();
});

it("redirects signed-out visitors from the dashboard to login", async () => {
  auth.status = AUTH_STATUS.UNAUTHENTICATED;
  auth.user = null;
  window.history.replaceState(null, "", routes.dashboard);
  renderRouter();

  await waitFor(() => expect(window.location.hash).toBe(routes.login));
  expect(screen.getByLabelText("Số Căn cước Công dân")).toBeTruthy();
});

it("keeps client validation in the form without calling login", () => {
  vi.mocked(auth.login).mockClear();
  window.history.replaceState(null, "", routes.login);
  renderRouter();
  const input = screen.getByLabelText("Số Căn cước Công dân");
  fireEvent.change(input, { target: { value: "123" } });
  fireEvent.submit(input.closest("form")!);
  expect(screen.getByRole("alert").closest("form")).not.toBeNull();
  expect(auth.login).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "Đóng thông báo" })).toBeNull();
});

it("shows restoration instead of the login form on a launch without a fragment", () => {
  auth.status = AUTH_STATUS.RESTORING;
  renderRouter();
  expect(screen.getByRole("status").textContent).toContain("khôi phục");
  expect(screen.queryByLabelText("Số Căn cước Công dân")).toBeNull();
});

it("redirects a restored session to the dashboard on a launch without a fragment", async () => {
  auth.status = AUTH_STATUS.AUTHENTICATED;
  renderRouter();
  await waitFor(() => expect(window.location.hash).toBe(routes.dashboard));
});

describe("loan application detail overlay", () => {
  const application = (id: string, status = "draft"): LoanApplication => ({
    id,
    applicationNo: `APP-${id}`,
    loanPackageId: "pkg-1",
    disbursementAccountId: "acc-1",
    requestedAmount: 50_000_000,
    termMonths: 12,
    status: status as LoanApplication["status"],
    documents: [],
    createdAt: "2026-10-30T00:00:00Z",
  });

  beforeEach(() => {
    auth.status = AUTH_STATUS.AUTHENTICATED;
    auth.token = "token";
    vi.mocked(getLoan).mockReset();
    vi.mocked(listLoans)
      .mockReset()
      .mockResolvedValueOnce({ items: [application("1")], nextCursor: "next" })
      .mockResolvedValueOnce({ items: [application("2")], nextCursor: null });
    window.history.replaceState(null, "", routes.loanApplications);
  });

  const openSecondPageDetail = async () => {
    renderRouter();
    fireEvent.click(await screen.findByRole("button", { name: "Xem thêm hồ sơ" }));
    const row = (await screen.findByText("APP-2")).closest("button")!;
    row.focus();
    fireEvent.click(row);
    return { row, dialog: await screen.findByRole("dialog", { name: "Chi tiết hồ sơ vay" }) };
  };

  it("keeps the loaded list and applies detail changes when closed", async () => {
    vi.mocked(getLoan).mockResolvedValue(application("2", "submitted"));
    await openSecondPageDetail();
    await screen.findByText("Thông tin hồ sơ");

    window.location.hash = routes.loanApplications;
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

    expect(listLoans).toHaveBeenCalledTimes(2);
    expect(screen.getByText("APP-1")).toBeTruthy();
    expect(screen.getByText("APP-2").closest("button")!.textContent).toContain("submitted");
  });

  it("isolates the list behind the dialog and returns focus on Escape", async () => {
    vi.mocked(getLoan).mockResolvedValue(application("2"));
    const { row, dialog } = await openSecondPageDetail();

    expect(row.closest("[inert]")).not.toBeNull();
    expect(document.activeElement).toBe(dialog);

    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(row.closest("[inert]")).toBeNull();
    expect(document.activeElement).toBe(row);
  });

  it("keeps Tab inside the dialog", async () => {
    vi.mocked(getLoan).mockResolvedValue(application("2"));
    const { dialog } = await openSecondPageDetail();
    await screen.findByText("Thông tin hồ sơ");
    const buttons = [...dialog.querySelectorAll<HTMLElement>("button:not([disabled]), input:not([disabled])")];
    const last = buttons[buttons.length - 1];

    fireEvent.keyDown(dialog, { key: "Tab", shiftKey: true });
    expect(document.activeElement).toBe(last);
    fireEvent.keyDown(last, { key: "Tab" });
    expect(document.activeElement).toBe(buttons[0]);
  });

  it("loads a directly opened detail above its list", async () => {
    vi.mocked(listLoans).mockReset().mockResolvedValue({ items: [application("1")], nextCursor: null });
    vi.mocked(getLoan).mockResolvedValue(application("1"));
    window.history.replaceState(null, "", loanApplication("1"));
    renderRouter();
    await screen.findByRole("dialog", { name: "Chi tiết hồ sơ vay" });
    await waitFor(() => expect(listLoans).toHaveBeenCalledTimes(1));
  });
});
