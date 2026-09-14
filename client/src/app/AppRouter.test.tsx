import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import {
  AuthContext,
  type AuthContextValue,
} from "@/features/auth/authContext";
import { AppRouter } from "./AppRouter";
import { routes } from "./routes";

const auth: AuthContextValue = {
  status: "unauthenticated",
  user: null,
  token: null,
  error: null,
  login: vi.fn(),
  logout: vi.fn(),
  refreshProfile: vi.fn(),
};
function renderRouter() {
  return render(
    <AuthContext.Provider value={auth}>
      <AppRouter />
    </AuthContext.Provider>,
  );
}
beforeEach(() => window.history.replaceState(null, "", "/"));
afterEach(() => {
  cleanup();
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
  await screen.findByRole("alert");
  expect(window.location.hash).toBe(routes.login);
  expect(screen.getByLabelText("Số Căn cước Công dân")).toBeTruthy();
});
