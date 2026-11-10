import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
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

function readStoredTheme(): AdminTheme | null {
  try {
    const stored = window.localStorage.getItem(ADMIN_THEME_STORAGE_KEY);
    return isAdminTheme(stored) ? stored : null;
  } catch {
    return null;
  }
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
  const hasExplicitTheme = useRef(readStoredTheme() !== null);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  useEffect(() => {
    if (!window.matchMedia) return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = (event: MediaQueryListEvent) => {
      if (hasExplicitTheme.current) return;
      setThemeState(event.matches ? "dark" : "light");
    };
    mediaQuery.addEventListener?.("change", handleChange);
    return () => mediaQuery.removeEventListener?.("change", handleChange);
  }, []);

  useEffect(() => {
    const handleStorage = (event: StorageEvent) => {
      if (event.key !== ADMIN_THEME_STORAGE_KEY) return;
      const nextTheme = isAdminTheme(event.newValue) ? event.newValue : null;
      hasExplicitTheme.current = nextTheme !== null;
      setThemeState(nextTheme ?? systemTheme());
    };
    window.addEventListener("storage", handleStorage);
    return () => window.removeEventListener("storage", handleStorage);
  }, []);

  const value = useMemo<ThemeContextValue>(
    () => ({
      theme,
      setTheme: (nextTheme) => {
        hasExplicitTheme.current = true;
        setThemeState(nextTheme);
        persistTheme(nextTheme);
      },
      toggleTheme: () => {
        const nextTheme = theme === "dark" ? "light" : "dark";
        hasExplicitTheme.current = true;
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
