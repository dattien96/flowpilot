import type { ThemePreference } from "./theme-store";

export const THEME_STORAGE_KEY = "flowpilot-theme-preference";

export const themeStorage = {
  getPreference(): ThemePreference {
    if (typeof window === "undefined" || typeof localStorage === "undefined" || typeof localStorage.getItem !== "function") {
      return "system";
    }
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === "light" || stored === "dark" || stored === "system") {
      return stored;
    }
    return "system";
  },
  setPreference(value: ThemePreference): void {
    if (typeof window === "undefined" || typeof localStorage === "undefined" || typeof localStorage.setItem !== "function") {
      return;
    }
    localStorage.setItem(THEME_STORAGE_KEY, value);
  },
};
