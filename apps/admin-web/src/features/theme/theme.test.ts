import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useThemeStore } from "./theme-store";
import { themeStorage } from "./theme-storage";

describe("Theme System Unit Tests", () => {
  let localStorageMock: Record<string, string> = {};

  beforeEach(() => {
    localStorageMock = {};
    vi.stubGlobal("localStorage", {
      getItem: vi.fn((key: string) => localStorageMock[key] || null),
      setItem: vi.fn((key: string, value: string) => {
        localStorageMock[key] = value;
      }),
      removeItem: vi.fn((key: string) => {
        delete localStorageMock[key];
      }),
      clear: vi.fn(() => {
        localStorageMock = {};
      }),
    });

    // Mock matchMedia
    vi.stubGlobal("window", {
      matchMedia: vi.fn((query: string) => ({
        matches: query.includes("dark") ? true : false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });

    // Mock documentElement classes
    const classList = {
      add: vi.fn(),
      remove: vi.fn(),
    };
    const style = {
      setProperty: vi.fn(),
    };
    vi.stubGlobal("document", {
      documentElement: {
        classList,
        style,
      },
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("persists light/dark/system states using canonical flowpilot-theme-preference key", () => {
    themeStorage.setPreference("dark");
    expect(localStorage.setItem).toHaveBeenCalledWith(
      "flowpilot-theme-preference",
      "dark"
    );
    expect(themeStorage.getPreference()).toBe("dark");

    themeStorage.setPreference("light");
    expect(themeStorage.getPreference()).toBe("light");

    themeStorage.setPreference("system");
    expect(themeStorage.getPreference()).toBe("system");
  });

  it("Zustand store updates preference state and applies theme class to document element", () => {
    const store = useThemeStore.getState();

    store.setPreference("dark");
    expect(useThemeStore.getState().preference).toBe("dark");
    expect(useThemeStore.getState().resolvedTheme).toBe("dark");
    expect(document.documentElement.classList.add).toHaveBeenCalledWith("dark");
    expect(document.documentElement.style.setProperty).toHaveBeenCalledWith(
      "color-scheme",
      "dark"
    );

    store.setPreference("light");
    expect(useThemeStore.getState().preference).toBe("light");
    expect(useThemeStore.getState().resolvedTheme).toBe("light");
    expect(document.documentElement.classList.remove).toHaveBeenCalledWith("dark");
    expect(document.documentElement.style.setProperty).toHaveBeenCalledWith(
      "color-scheme",
      "light"
    );
  });
});
