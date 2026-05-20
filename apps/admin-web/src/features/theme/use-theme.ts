import { useThemeStore } from "./theme-store";
import type { ThemePreference, ResolvedTheme } from "./theme-store";

export function useTheme(): {
  preference: ThemePreference;
  resolvedTheme: ResolvedTheme;
  setPreference: (value: ThemePreference) => void;
} {
  const preference = useThemeStore((state) => state.preference);
  const resolvedTheme = useThemeStore((state) => state.resolvedTheme);
  const setPreference = useThemeStore((state) => state.setPreference);

  return {
    preference,
    resolvedTheme,
    setPreference,
  };
}
