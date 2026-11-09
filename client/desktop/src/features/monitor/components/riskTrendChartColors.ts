import type { AdminTheme } from "@/shared/theme/theme";

export function getChartColors(theme: AdminTheme) {
  return theme === "dark"
    ? {
        border: "#b4c5ff",
        fill: "rgba(37, 99, 235, 0.13)",
        grid: "#272b33",
        muted: "#8d90a0",
        danger: "#d52022",
        surface: "#2a2a2c",
        markerGlow: "rgba(180, 197, 255, 0.22)",
        hoverGuide: "rgba(180, 197, 255, 0.75)",
      }
    : {
        border: "#2563eb",
        fill: "rgba(37, 99, 235, 0.12)",
        grid: "#d6dee9",
        muted: "#64748b",
        danger: "#b91c1c",
        surface: "#ffffff",
        markerGlow: "rgba(37, 99, 235, 0.18)",
        hoverGuide: "rgba(37, 99, 235, 0.45)",
      };
}
