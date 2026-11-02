import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { routes } from "@/app/routes";
import {
  confirmPasswordReset,
  requestPasswordReset,
} from "@/features/auth/api/auth";
import { ApiError } from "@/shared/lib/http";
import { ForgotPasswordView } from "./ForgotPasswordView";

vi.mock("@/features/auth/api/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/auth/api/auth")>()),
  requestPasswordReset: vi.fn(async () => undefined),
  confirmPasswordReset: vi.fn(async () => undefined),
}));

const strongPassword = ["New", "passw0rd", "!"].join("");

const type = (label: string, value: string) =>
  fireEvent.change(screen.getByLabelText(label), { target: { value } });

async function reachConfirmStep() {
  render(<ForgotPasswordView />);
  type("Số Căn cước Công dân", "012345678901");
  type("Email đã đăng ký", " owner@example.com ");
  fireEvent.click(screen.getByRole("button", { name: "Gửi mã xác thực" }));
  await screen.findByLabelText("Mã xác thực");
}

function fillConfirmStep() {
  type("Mã xác thực", "123456");
  type("Mật khẩu mới", strongPassword);
  type("Xác nhận mật khẩu mới", strongPassword);
}

const submitButton = () =>
  screen.getByRole<HTMLButtonElement>("button", { name: "Đặt lại mật khẩu" });

beforeEach(() => {
  vi.clearAllMocks();
  window.location.hash = routes.forgotPassword;
});

afterEach(cleanup);

it("keeps the send button disabled until CCCD and email are valid", () => {
  render(<ForgotPasswordView />);
  const send = screen.getByRole<HTMLButtonElement>("button", {
    name: "Gửi mã xác thực",
  });
  type("Số Căn cước Công dân", "123");
  type("Email đã đăng ký", "owner@example.com");
  expect(send.disabled).toBe(true);
  type("Số Căn cước Công dân", "012345678901");
  expect(send.disabled).toBe(false);
});

it("shows a message that does not confirm the account exists", async () => {
  await reachConfirmStep();
  expect(requestPasswordReset).toHaveBeenCalledWith({
    cccdNumber: "012345678901",
    email: "owner@example.com",
  });
  expect(screen.getByRole("status").textContent).toContain(
    "Nếu thông tin khớp",
  );
});

it("resets the password and returns to login", async () => {
  await reachConfirmStep();
  fillConfirmStep();
  fireEvent.click(submitButton());
  await vi.waitFor(() => expect(window.location.hash).toBe(routes.login));
  expect(confirmPasswordReset).toHaveBeenCalledWith({
    cccdNumber: "012345678901",
    otp: "123456",
    newPassword: strongPassword,
  });
});

it("clears secrets and stays on the form when the code is rejected", async () => {
  vi.mocked(confirmPasswordReset).mockRejectedValueOnce(
    new ApiError(400, "bad"),
  );
  await reachConfirmStep();
  fillConfirmStep();
  fireEvent.click(submitButton());
  await screen.findByText(/Mã xác thực không đúng/);
  expect(screen.getByLabelText<HTMLInputElement>("Mã xác thực").value).toBe("");
  expect(screen.getByLabelText<HTMLInputElement>("Mật khẩu mới").value).toBe(
    "",
  );
  expect(window.location.hash).toBe(routes.forgotPassword);
});

it("requires a strong password that matches its confirmation", async () => {
  await reachConfirmStep();
  fillConfirmStep();
  type("Xác nhận mật khẩu mới", `${strongPassword}x`);
  expect(submitButton().disabled).toBe(true);
  type("Mật khẩu mới", "weakpass");
  type("Xác nhận mật khẩu mới", "weakpass");
  expect(submitButton().disabled).toBe(true);
});

it("blocks resending during the cooldown", async () => {
  await reachConfirmStep();
  const resend = screen.getByRole<HTMLButtonElement>("button", {
    name: /Gửi lại mã sau/,
  });
  expect(resend.disabled).toBe(true);
});
