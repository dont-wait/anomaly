import { render, screen, waitFor } from "@testing-library/react";
import { LoginPage } from "./LoginPage";
import { ThemeProvider } from "@/shared/theme/ThemeProvider";

describe("LoginPage theme presentation", () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it.each([
    ["light", "Chuyển sang giao diện tối"],
    ["dark", "Chuyển sang giao diện sáng"],
  ] as const)(
    "renders the main login console in %s theme",
    async (theme, nextLabel) => {
      window.localStorage.setItem("anomaly.admin.theme", theme);
      render(
        <ThemeProvider>
          <LoginPage />
        </ThemeProvider>,
      );
      await waitFor(() =>
        expect(document.documentElement.dataset.theme).toBe(theme),
      );
      expect(
        screen.getByRole("heading", { name: "Đăng nhập quản trị" }),
      ).toBeTruthy();
      expect(screen.getByRole("button", { name: nextLabel })).toBeTruthy();
      expect(document.documentElement.style.colorScheme).toBe(theme);
    },
  );
});
