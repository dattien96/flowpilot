import type { IdeBridge } from "@/types/contract";

// Prefers the Electron preload bridge (whose main-side handler invokes the real
// IDE CLI in Part B — code -g / cursor / studio / xed); falls back to console.log
// so the renderer also works in a plain browser tab during UI development. This
// renderer code is unchanged between Part A and Part B — only the main-side handler
// swapped from a stub to real invocation.
export const ideBridge: IdeBridge = {
  async openInIde(file: string, line?: number): Promise<void> {
    const bridge = window as Window & { flowpilot?: { openInIde?: (file: string, line?: number) => Promise<void>; openExternal?: (url: string) => Promise<void> } };
    if (bridge.flowpilot?.openInIde) {
      await bridge.flowpilot.openInIde(file, line);
      return;
    }
    // eslint-disable-next-line no-console
    console.log("[IdeBridge stub:renderer] open in IDE:", { file, line });
  },

  async openExternal(url: string): Promise<void> {
    const bridge = window as Window & { flowpilot?: { openInIde?: (file: string, line?: number) => Promise<void>; openExternal?: (url: string) => Promise<void> } };
    if (bridge.flowpilot?.openExternal) {
      await bridge.flowpilot.openExternal(url);
      return;
    }
    // Browser fallback (renderer running in a plain tab during dev).
    window.open(url, "_blank", "noopener");
  },
};
