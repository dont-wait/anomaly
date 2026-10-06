import { fireEvent, render, screen } from "@testing-library/react";
import { AdminShell } from "./AdminShell";
import { routes } from "@/app/routes";

const user = {
  id: "admin-id",
  accountNo: "ACC-ADMIN",
  username: "admin.staff",
  fullName: "Admin Staff",
  email: "admin@example.com",
  role: "admin",
};

describe("AdminShell", () => {
  beforeEach(() => {
    window.location.hash = routes.monitor;
  });

  it("navigates between protected sections", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    fireEvent.click(screen.getByRole("button", { name: /cảnh báo/i }));

    expect(window.location.hash).toBe(routes.alerts);
  });

  it("renders the Stitch navigation groups", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    expect(
      screen.getByRole("button", { name: /Tài khoản rủi ro/ }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Replay Console/ })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /Thẩm định hồ sơ/ }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Duyệt KYC/ })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /Giao dịch & Rủi ro/ }),
    ).toBeTruthy();
  });

  it("renders the signed-in admin profile", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    expect(screen.getAllByText("Admin Staff")).toHaveLength(2);
    expect(screen.getByText("Administrator · ACC-ADMIN")).toBeTruthy();
    expect(
      screen.getAllByRole("img", { name: "Tài khoản Admin Staff" }),
    ).toHaveLength(2);
  });

  it("toggles the sidebar from the operator row", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Thu gọn sidebar" }));

    const expandButton = screen.getByRole("button", {
      name: "Mở rộng sidebar",
    });
    expect(expandButton.getAttribute("aria-pressed")).toBe("true");
    const avatars = screen.getAllByRole("img", {
      name: "Tài khoản Admin Staff",
    });
    expect(avatars).toHaveLength(2);
    expect(avatars[0]?.className).toContain("hidden");
    expect(avatars[1]?.className).not.toContain("hidden");
    expect(
      screen.queryByRole("button", { name: "Thu gọn sidebar" }),
    ).toBeNull();
  });

  it("calls the logout handler", () => {
    const onLogout = vi.fn();
    render(
      <AdminShell currentRoute={routes.monitor} user={user} onLogout={onLogout}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Đăng xuất" }));

    expect(onLogout).toHaveBeenCalledOnce();
  });
});
