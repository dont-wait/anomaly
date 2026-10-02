import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { LoginPage } from "./LoginPage";

describe("LoginPage", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        const body = JSON.parse(String(init?.body)) as {
          cccdNumber: string;
          password: string;
        };
        if (body.password === "wrong-password") {
          return new Response(
            JSON.stringify({
              status: 401,
              title: "Unauthorized",
              errors: [
                {
                  code: "INVALID_CREDENTIALS",
                  detail: "Thông tin đăng nhập không chính xác.",
                },
              ],
            }),
            { status: 401 },
          );
        }
        return new Response(
          JSON.stringify({
            status: 200,
            message: "Login successful",
            data: {
              token: "admin-token",
              expiresAt: "2030-01-01T00:00:00Z",
              user: {
                id: "admin-id",
                accountNo: "ACC-ADMIN",
                fullName: "Admin Staff",
                role: body.cccdNumber === "079123456789" ? "user" : "admin",
              },
            },
          }),
          { status: 200 },
        );
      }),
    );
    localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

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

  it("rejects a valid normal-user account", async () => {
    render(<LoginPage />);

    fireEvent.change(screen.getByLabelText("Số CCCD"), {
      target: { value: "079123456789" },
    });
    fireEvent.change(screen.getByLabelText("Mật khẩu"), {
      target: { value: "DemoLocal@123" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Đăng nhập" }));

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("không có quyền truy cập");
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
