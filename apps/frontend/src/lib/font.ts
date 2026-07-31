import { ref } from "vue";
import { safeGetItem, safeSetItem } from "@/lib/storage";

type FontFamily = "ibm-plex-sans" | "inter" | "fira-sans";

type FontOption = {
  value: FontFamily;
  label: string;
  css: string;
};

export const fontOptions: FontOption[] = [
  { value: "ibm-plex-sans", label: "IBM Plex Sans", css: '"IBM Plex Sans", system-ui, sans-serif' },
  { value: "inter", label: "Inter", css: '"Inter", system-ui, sans-serif' },
  { value: "fira-sans", label: "Fira Sans", css: '"Fira Sans", system-ui, sans-serif' },
];

const FONT_KEY = "font";
const current = ref<FontFamily>("inter");
let initialized = false;

const fontMap: Record<FontFamily, string> = Object.fromEntries(
  fontOptions.map((f) => [f.value, f.css]),
) as Record<FontFamily, string>;

const googleFontNames: Record<FontFamily, string> = {
  "ibm-plex-sans": "IBM+Plex+Sans",
  inter: "Inter",
  "fira-sans": "Fira+Sans",
};

const loaded = new Set<FontFamily>();

function buildGoogleFontsUrl(family: string): string {
  const weights = [300, 400, 500, 600, 700];
  const wght = weights.map((w) => `0,${w}`).join(";");
  return `https://fonts.googleapis.com/css2?family=${family}:ital,wght@${wght}&display=swap`;
}

function loadFont(f: FontFamily): Promise<void> {
  if (loaded.has(f)) return Promise.resolve();

  return new Promise((resolve) => {
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.href = buildGoogleFontsUrl(googleFontNames[f]);
    link.onload = () => {
      loaded.add(f);
      resolve();
    };
    link.onerror = () => resolve();
    document.head.appendChild(link);
  });
}

function applyFont() {
  document.documentElement.style.setProperty("--font-sans", fontMap[current.value]);
  safeSetItem(FONT_KEY, current.value);
}

export function initFont() {
  if (initialized || typeof window === "undefined") return;
  initialized = true;

  const stored = safeGetItem(FONT_KEY) as FontFamily | null;
  current.value = stored && Object.hasOwn(fontMap, stored) ? stored : "inter";

  loadFont(current.value).then(applyFont);
}

function setFont(f: FontFamily) {
  current.value = f;
  loadFont(f).then(applyFont);
}

export function useFont() {
  initFont();
  return { current, setFont, fontOptions };
}
