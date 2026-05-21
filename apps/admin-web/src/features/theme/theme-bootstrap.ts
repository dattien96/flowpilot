import { THEME_STORAGE_KEY } from "./theme-storage";
import type { ThemePreference, ResolvedTheme } from "./theme-store";

export function resolveTheme(preference: ThemePreference): ResolvedTheme {
  if (preference === "system") {
    if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
      const isDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      return isDark ? "dark" : "light";
    }
    return "light";
  }
  return preference;
}

export function initThemeHtml(): void {
  if (typeof window === "undefined") return;
  const stored = localStorage.getItem(THEME_STORAGE_KEY) as ThemePreference | null;
  const preference: ThemePreference = stored === "light" || stored === "dark" || stored === "system" ? stored : "system";
  const resolved = resolveTheme(preference);
  
  const doc = document.documentElement;
  if (resolved === "dark") {
    doc.classList.add("dark");
  } else {
    doc.classList.remove("dark");
  }
}
