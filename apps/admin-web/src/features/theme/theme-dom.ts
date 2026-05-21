import type { ResolvedTheme } from "./theme-store";

export const themeDom = {
  applyTheme(theme: ResolvedTheme): void {
    if (typeof window === "undefined" || typeof document === "undefined" || !document.documentElement) return;
    const doc = document.documentElement;
    if (theme === "dark") {
      doc.classList.add("dark");
      doc.style.setProperty("color-scheme", "dark");
    } else {
      doc.classList.remove("dark");
      doc.style.setProperty("color-scheme", "light");
    }
  },
};
