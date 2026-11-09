import { render, screen, waitFor } from "@testing-library/react";
import { AdminShell } from "./AdminShell";
import { AdminAlertCountContext } from "@/features/alerts/adminAlertCountContext";
import { routes } from "@/app/routes";
import { ThemeProvider } from "@/shared/theme/ThemeProvider";

const user = {
  id: "admin-id",
  accountNo: "ACC-ADMIN",
  username: "admin.staff",
  fullName: "Admin Staff",
  email: "admin@example.com",
  role: "admin",
};

describe("authenticated admin shell theme presentation", () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it.each([
    ["light", "Chuyển sang giao diện tối"],
    ["dark", "Chuyển sang giao diện sáng"],
  ] as const)(
    "renders the shell and dashboard slot in %s theme",
    async (theme, nextLabel) => {
      window.localStorage.setItem("anomaly.admin.theme", theme);
      render(
        <ThemeProvider>
          <AdminAlertCountContext.Provider value={0}>
            <AdminShell currentRoute={routes.monitor} user={user}>
              <section aria-label="Dashboard test view">
                Dashboard content
              </section>
            </AdminShell>
          </AdminAlertCountContext.Provider>
        </ThemeProvider>,
      );
      await waitFor(() =>
        expect(document.documentElement.dataset.theme).toBe(theme),
      );
      expect(screen.getByRole("complementary")).toBeTruthy();
      expect(
        screen.getByRole("region", { name: "Dashboard test view" }),
      ).toBeTruthy();
      expect(screen.getByRole("button", { name: nextLabel })).toBeTruthy();
    },
  );
});
