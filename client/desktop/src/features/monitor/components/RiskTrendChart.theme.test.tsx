import { render, screen, waitFor } from "@testing-library/react";
import { ThemeProvider } from "@/shared/theme/ThemeProvider";
import { RiskTrendChart } from "./RiskTrendChart";
import { getChartColors } from "./riskTrendChartColors";

vi.mock("react-chartjs-2", () => ({
  Line: ({
    data,
    options,
  }: {
    data: { datasets: Array<{ borderColor: string }> };
    options: { scales?: { y?: { grid?: { color?: string } } } };
  }) => (
    <div
      data-testid="risk-line"
      data-border={data.datasets[0]?.borderColor}
      data-grid={options.scales?.y?.grid?.color}
    />
  ),
}));

const series = [
  {
    timestamp: "2026-11-09T00:00:00Z",
    assessmentCount: 4,
    anomalousCount: 1,
    averageRiskScore: 0.42,
  },
];

describe("RiskTrendChart theme presentation", () => {
  afterEach(() => {
    window.localStorage.clear();
    document.documentElement.removeAttribute("data-theme");
  });

  it.each([
    ["light", "#2563eb", "#d6dee9"],
    ["dark", "#b4c5ff", "#272b33"],
  ] as const)(
    "renders actual chart colors for %s theme",
    async (theme, border, grid) => {
      window.localStorage.setItem("anomaly.admin.theme", theme);
      render(
        <ThemeProvider>
          <RiskTrendChart series={series} />
        </ThemeProvider>,
      );
      await waitFor(() =>
        expect(document.documentElement.dataset.theme).toBe(theme),
      );
      const chart = screen.getByTestId("risk-line");
      expect(chart.dataset.border).toBe(border);
      expect(chart.dataset.grid).toBe(grid);
      expect(getChartColors(theme).markerGlow).not.toBe(
        getChartColors(theme === "dark" ? "light" : "dark").markerGlow,
      );
      expect(getChartColors(theme).hoverGuide).not.toBe(
        getChartColors(theme === "dark" ? "light" : "dark").hoverGuide,
      );
    },
  );
});
