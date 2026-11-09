import { fireEvent, render, screen } from "@testing-library/react";
import { AdminShell } from "./AdminShell";
import { routes } from "@/app/routes";
import { AdminAlertCountContext } from "@/features/alerts/adminAlertCountContext";

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

  it("shows the unhandled alert count next to Cảnh báo and caps it at 99+", () => {
    render(
      <AdminAlertCountContext.Provider value={123}>
        <AdminShell currentRoute={routes.monitor} user={user}>
          <div>Dashboard content</div>
        </AdminShell>
      </AdminAlertCountContext.Provider>,
    );

    expect(screen.getByText("99+")).toBeTruthy();
    expect(screen.getByRole("button", { name: /cảnh báo/i })).toBeTruthy();
  });

  it("renders the Stitch navigation groups", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    expect(
      screen.queryByRole("button", { name: /Tài khoản rủi ro/ }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: /Replay Console/ })).toBeNull();
    expect(
      screen.getByRole("button", { name: /Thẩm định hồ sơ/ }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Duyệt KYC/ })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /Giao dịch & Rủi ro/ }),
    ).toBeTruthy();
  });

  it("keeps the sidebar fixed to the viewport while its menu scrolls", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    expect(screen.getByRole("complementary").className).toContain("sticky");
    expect(screen.getByRole("complementary").className).toContain("top-0");
    expect(screen.getByRole("complementary").className).toContain("h-dvh");
    expect(
      screen.getByRole("navigation", { name: "Menu admin" }).className,
    ).toContain("overflow-y-auto");
    expect(
      screen.getByRole("navigation", { name: "Menu admin" }).className,
    ).toContain("min-h-0");
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

  it("does not render environment and assignment actions in the header", () => {
    render(
      <AdminShell currentRoute={routes.monitor} user={user}>
        <div>Dashboard content</div>
      </AdminShell>,
    );

    expect(screen.queryByText("PROD · V0.4")).toBeNull();
    expect(screen.queryByText("UTC+7")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Nhận việc tiếp theo" }),
    ).toBeNull();
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
