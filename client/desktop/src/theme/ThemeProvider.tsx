import { useEffect, useMemo, useState, type ReactNode } from "react";
import {
  ADMIN_THEME_STORAGE_KEY,
  ThemeContext,
  type AdminTheme,
  type ThemeContextValue,
} from "./theme";

function isAdminTheme(value: string | null): value is AdminTheme {
  return value === "light" || value === "dark";
}

function systemTheme(): AdminTheme {
  if (typeof window === "undefined") return "dark";
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

function readInitialTheme(): AdminTheme {
  try {
    const stored = window.localStorage.getItem(ADMIN_THEME_STORAGE_KEY);
    if (isAdminTheme(stored)) return stored;
  } catch {
    // Storage may be unavailable in a restricted browser context.
  }
  return systemTheme();
}

function applyTheme(theme: AdminTheme) {
  const root = document.documentElement;
  root.dataset.theme = theme;
  root.style.colorScheme = theme;
  const themeColor = document.querySelector<HTMLMetaElement>(
    'meta[name="theme-color"]',
  );
  if (themeColor) themeColor.content = theme === "dark" ? "#09090b" : "#f6f8fc";
}

function persistTheme(theme: AdminTheme) {
  try {
    window.localStorage.setItem(ADMIN_THEME_STORAGE_KEY, theme);
  } catch {
    // The theme still applies for the current session when storage is blocked.
  }
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<AdminTheme>(readInitialTheme);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    let hasStoredTheme = false;
    try {
      hasStoredTheme = isAdminTheme(
        window.localStorage.getItem(ADMIN_THEME_STORAGE_KEY),
      );
    } catch {
      // Fall back to the system preference when storage is unavailable.
    }
    if (hasStoredTheme || !window.matchMedia) return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = (event: MediaQueryListEvent) => {
      setThemeState(event.matches ? "dark" : "light");
    };
    mediaQuery.addEventListener?.("change", handleChange);
    return () => mediaQuery.removeEventListener?.("change", handleChange);
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({
      theme,
      setTheme: (nextTheme) => {
        setThemeState(nextTheme);
        persistTheme(nextTheme);
      },
      toggleTheme: () => {
        const nextTheme = theme === "dark" ? "light" : "dark";
        setThemeState(nextTheme);
        persistTheme(nextTheme);
      },
    }),
    [theme],
  );

  return (
    <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
  );
}
