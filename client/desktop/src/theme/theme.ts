import { createContext, useContext } from "react";

export type AdminTheme = "light" | "dark";

export const ADMIN_THEME_STORAGE_KEY = "anomaly.admin.theme";

export interface ThemeContextValue {
  theme: AdminTheme;
  setTheme: (theme: AdminTheme) => void;
  toggleTheme: () => void;
}

const fallbackThemeContext: ThemeContextValue = {
  theme: "dark",
  setTheme: () => undefined,
  toggleTheme: () => undefined,
};

export const ThemeContext =
  createContext<ThemeContextValue>(fallbackThemeContext);

export function useAdminTheme() {
  return useContext(ThemeContext);
}
