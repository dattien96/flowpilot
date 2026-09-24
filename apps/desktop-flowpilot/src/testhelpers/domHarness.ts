import { JSDOM } from "jsdom";

// KR-005 component-test harness: the phase1 suite runs compiled JS under
// `node --test` — no DOM. This installs the minimal browser globals a React
// component render needs (document, events, timers) and returns a restore
// function so suites stay isolated. Additive infra only — nothing here runs
// in production builds.

const DOM_GLOBALS = [
  "window",
  "document",
  "HTMLElement",
  "Element",
  "Node",
  "SVGElement",
  "DocumentFragment",
  "Event",
  "CustomEvent",
  "KeyboardEvent",
  "MouseEvent",
  "PointerEvent",
  "FocusEvent",
  "InputEvent",
  "HTMLButtonElement",
  "HTMLDivElement",
  "HTMLUListElement",
  "HTMLLIElement",
  "HTMLSpanElement",
  "HTMLInputElement",
  "HTMLTextAreaElement",
  "MutationObserver",
  "getComputedStyle",
] as const;

export function setupDom(): () => void {
  const dom = new JSDOM("<!doctype html><html><body></body></html>", {
    url: "http://localhost/",
    pretendToBeVisual: true,
  });
  const g = globalThis as Record<string | symbol, unknown>;
  const saved = new Map<string, unknown>();
  const had = new Map<string, boolean>();
  for (const key of DOM_GLOBALS) {
    had.set(key, key in globalThis);
    saved.set(key, g[key]);
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value: (dom.window as unknown as Record<string, unknown>)[key],
    });
  }
  // React act() requires this flag outside a real test runner env.
  g.IS_REACT_ACT_ENVIRONMENT = true;
  let restored = false;
  return () => {
    if (restored) return;
    restored = true;
    for (const key of DOM_GLOBALS) {
      if (had.get(key)) {
        Object.defineProperty(globalThis, key, {
          configurable: true,
          writable: true,
          value: saved.get(key),
        });
      } else {
        delete g[key];
      }
    }
    delete g.IS_REACT_ACT_ENVIRONMENT;
  };
}
