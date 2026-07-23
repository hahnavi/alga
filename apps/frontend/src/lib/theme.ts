import { ref } from "vue";
import { safeGetItem, safeSetItem } from "@/lib/storage";

export type ThemeMode = "system" | "dark" | "light";

const THEME_KEY = "theme";
const isDark = ref(true);
const mode = ref<ThemeMode>("system");
let initialized = false;

function getSystemPrefersDark(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

function resolveIsDark(m: ThemeMode): boolean {
  if (m === "system") return getSystemPrefersDark();
  return m === "dark";
}

function applyTheme() {
  const dark = resolveIsDark(mode.value);
  isDark.value = dark;
  if (dark) {
    document.documentElement.classList.remove("light");
  } else {
    document.documentElement.classList.add("light");
  }
  safeSetItem(THEME_KEY, mode.value);
}

let mediaQuery: MediaQueryList | null = null;

function onSystemThemeChange() {
  if (mode.value === "system") {
    applyTheme();
  }
}

function initTheme() {
  if (initialized || typeof window === "undefined") return;
  initialized = true;

  const stored = safeGetItem(THEME_KEY) as ThemeMode | null;
  mode.value = stored === "dark" || stored === "light" || stored === "system" ? stored : "system";

  applyTheme();

  mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
  mediaQuery.addEventListener("change", onSystemThemeChange);
}

function setMode(m: ThemeMode) {
  mode.value = m;
  applyTheme();
}

export function useTheme() {
  initTheme();

  function toggle() {
    if (mode.value === "dark") {
      setMode("light");
    } else {
      setMode("dark");
    }
  }

  return { isDark, mode, setMode, toggle };
}
