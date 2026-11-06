import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { ThemeProvider } from "./ThemeProvider";
import { ThemeToggle } from "./ThemeToggle";

describe("admin theme", () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
    document.documentElement.style.colorScheme = "";
  });

  it("restores the selected theme and persists a toggle", async () => {
    window.localStorage.setItem("anomaly.admin.theme", "dark");

    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );

    const toggle = screen.getByRole("button", {
      name: "Chuyển sang giao diện sáng",
    });
    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("dark"),
    );

    fireEvent.click(toggle);

    expect(document.documentElement.dataset.theme).toBe("light");
    expect(window.localStorage.getItem("anomaly.admin.theme")).toBe("light");
    expect(
      screen.getByRole("button", { name: "Chuyển sang giao diện tối" }),
    ).toBeTruthy();
  });

  it("uses the system preference when no theme has been saved", async () => {
    const mediaQuery = {
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    } as unknown as MediaQueryList;
    vi.stubGlobal(
      "matchMedia",
      vi.fn(() => mediaQuery),
    );

    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );

    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("dark"),
    );
    expect(
      screen.getByRole("button", { name: "Chuyển sang giao diện sáng" }),
    ).toBeTruthy();

    vi.unstubAllGlobals();
  });
});
