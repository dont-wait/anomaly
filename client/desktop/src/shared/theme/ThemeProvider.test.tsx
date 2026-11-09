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
    window.localStorage.setItem("anomaly.admin.theme", "light");

    render(
      <ThemeProvider>
        <ThemeToggle />
      </ThemeProvider>,
    );

    const toggle = screen.getByRole("button", {
      name: "Chuyển sang giao diện tối",
    });
    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("light"),
    );

    fireEvent.click(toggle);

    expect(document.documentElement.dataset.theme).toBe("dark");
    expect(window.localStorage.getItem("anomaly.admin.theme")).toBe("dark");
    expect(
      screen.getByRole("button", { name: "Chuyển sang giao diện sáng" }),
    ).toBeTruthy();
  });

  it("uses the system preference when no theme has been saved", async () => {
    const mediaQuery = {
      matches: false,
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
      expect(document.documentElement.dataset.theme).toBe("light"),
    );
    expect(
      screen.getByRole("button", { name: "Chuyển sang giao diện tối" }),
    ).toBeTruthy();

    vi.unstubAllGlobals();
  });

  it("follows system changes only until the user explicitly chooses a theme", async () => {
    let listener: ((event: MediaQueryListEvent) => void) | undefined;
    const mediaQuery = {
      matches: false,
      addEventListener: vi.fn(
        (_type: string, next: (event: MediaQueryListEvent) => void) => {
          listener = next;
        },
      ),
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
      expect(document.documentElement.dataset.theme).toBe("light"),
    );
    listener?.({ matches: true } as MediaQueryListEvent);
    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("dark"),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Chuyển sang giao diện sáng" }),
    );
    expect(document.documentElement.dataset.theme).toBe("light");
    listener?.({ matches: true } as MediaQueryListEvent);
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(document.documentElement.dataset.theme).toBe("light");
  });

  it("synchronizes a theme change from another tab and returns to system mode when cleared", async () => {
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
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "anomaly.admin.theme",
        newValue: "light",
      }),
    );
    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("light"),
    );
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "anomaly.admin.theme",
        newValue: null,
      }),
    );
    await waitFor(() =>
      expect(document.documentElement.dataset.theme).toBe("dark"),
    );
  });
});
