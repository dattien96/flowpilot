import { create } from "zustand";
import { themeStorage } from "./theme-storage";
import { resolveTheme } from "./theme-bootstrap";
import { themeDom } from "./theme-dom";

export type ThemePreference = "light" | "dark" | "system";
export type ResolvedTheme = "light" | "dark";

export interface ThemeState {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (value: ThemePreference) => void;
  onSystemPreferenceChange: () => void;
}

const initialPreference = themeStorage.getPreference();
const initialResolved = resolveTheme(initialPreference);

// Initialize DOM synchronously on load
themeDom.applyTheme(initialResolved);

export const useThemeStore = create<ThemeState>((set) => ({
  preference: initialPreference,
  resolvedTheme: initialResolved,
  setPreference: (preference) => {
    themeStorage.setPreference(preference);
    const resolved = resolveTheme(preference);
    themeDom.applyTheme(resolved);
    set({ preference, resolvedTheme: resolved });
  },
  onSystemPreferenceChange: () => {
    set((state) => {
      if (state.preference !== "system") {
        return state;
      }
      const resolved = resolveTheme("system");
      themeDom.applyTheme(resolved);
      return { resolvedTheme: resolved };
    });
  },
}));
