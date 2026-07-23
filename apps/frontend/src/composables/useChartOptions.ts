import { computed } from "vue";
import type { ChartOptions } from "chart.js";
import { useTheme } from "@/lib/theme";

function readCssVars(...names: string[]): Record<string, string> {
  const style = getComputedStyle(document.documentElement);
  const result: Record<string, string> = {};
  for (const name of names) {
    result[name] = style.getPropertyValue(name).trim();
  }
  return result;
}

export function useChartOptions() {
  const { isDark } = useTheme();

  // Read theme so this computed re-evaluates when dark mode flips; the
  // CSS variables themselves are static, but downstream consumers pick
  // grid color via isDark below.
  const colors = computed(() => {
    void isDark.value;
    return readCssVars(
      "--chart-blue",
      "--chart-green",
      "--chart-red",
      "--chart-amber",
      "--chart-gray",
      "--chart-blue-fill",
      "--chart-green-fill",
      "--chart-red-fill",
      "--chart-amber-fill",
      "--chart-grid-dark",
      "--chart-grid-light",
    );
  });

  const lineChartOptions = computed<ChartOptions<"line">>(() => {
    const vars = readCssVars("--text-secondary", "--text-muted");
    const textSecondary = vars["--text-secondary"];
    const textMuted = vars["--text-muted"];
    const gridColor = isDark.value
      ? colors.value["--chart-grid-dark"]
      : colors.value["--chart-grid-light"];
    return {
      responsive: true,
      maintainAspectRatio: false,
      interaction: { mode: "index", intersect: false },
      scales: {
        x: {
          ticks: { color: textMuted, font: { size: 11 } },
          grid: { color: gridColor },
        },
        y: {
          beginAtZero: true,
          ticks: { color: textMuted, font: { size: 11 } },
          grid: { color: gridColor },
        },
      },
      plugins: {
        legend: {
          labels: {
            color: textSecondary,
            usePointStyle: true,
            pointStyleWidth: 10,
            font: { size: 12 },
          },
        },
        tooltip: {
          backgroundColor: "rgba(0,0,0,0.8)",
          titleColor: "#fff",
          bodyColor: "#fff",
          padding: 10,
          cornerRadius: 6,
        },
      },
    };
  });

  const doughnutChartOptions = computed<ChartOptions<"doughnut">>(() => {
    const vars = readCssVars("--text-secondary");
    const textSecondary = vars["--text-secondary"];
    return {
      responsive: true,
      maintainAspectRatio: false,
      cutout: "65%",
      plugins: {
        legend: {
          position: "bottom",
          labels: {
            color: textSecondary,
            padding: 16,
            usePointStyle: true,
            pointStyleWidth: 10,
            font: { size: 12 },
          },
        },
        tooltip: {
          backgroundColor: "rgba(0,0,0,0.8)",
          titleColor: "#fff",
          bodyColor: "#fff",
          padding: 10,
          cornerRadius: 6,
        },
      },
    };
  });

  return { lineChartOptions, doughnutChartOptions, colors };
}
