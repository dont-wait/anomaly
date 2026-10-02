import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  it("shows required errors without submitting", () => {
    render(<LoginPage />);

    fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));

    expect(screen.getByText("Số CCCD phải gồm đúng 12 chữ số.")).toBeTruthy();
    expect(screen.getByText("Vui lòng nhập mật khẩu.")).toBeTruthy();
  });

  it("shows an error for invalid credentials", async () => {
    render(<LoginPage />);

    fireEvent.change(screen.getByLabelText("Số CCCD"), {
      target: { value: "123456789012" },
    });
    fireEvent.change(screen.getByLabelText("Mật khẩu"), {
      target: { value: "wrong-password" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("Thông tin đăng nhập không chính xác");
  });

  it("navigates to the monitor placeholder after a successful login", async () => {
    window.location.hash = "#/admin/login";
    render(<LoginPage />);

    fireEvent.change(screen.getByLabelText("Số CCCD"), {
      target: { value: "001234567890" },
    });
    fireEvent.change(screen.getByLabelText("Mật khẩu"), {
      target: { value: "admin123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));

    await waitFor(() => {
      expect(window.location.hash).toBe("#/admin/monitor");
    });
  });
});
