import { useEffect } from "react";
import { useThemeStore } from "./theme-store";

export function ThemeController(): null {
  const onSystemPreferenceChange = useThemeStore((state) => state.onSystemPreferenceChange);

  useEffect(() => {
    if (typeof window === "undefined") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    
    // Modern event listener syntax
    const listener = () => {
      onSystemPreferenceChange();
    };

    // Attach listener
    mediaQuery.addEventListener("change", listener);

    // Initial check/sync
    onSystemPreferenceChange();

    return () => {
      mediaQuery.removeEventListener("change", listener);
    };
  }, [onSystemPreferenceChange]);

  return null;
}
