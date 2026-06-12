import type { IdeBridge } from "@/types/contract";

declare global {
  interface Window {
    flowpilot?: {
      openInIde(file: string, line?: number): Promise<{ ok: boolean; stub?: boolean }>;
      openExternal(url: string): Promise<{ ok: boolean }>;
    };
  }
}

// Part A stub. Prefers the Electron preload bridge (which itself just logs in
// Part A); falls back to console.log so the renderer also works in a plain
// browser tab during UI development. Part B swaps the main-side handler for real
// IDE CLI invocation — this renderer code does not change.
export const ideBridge: IdeBridge = {
  async openInIde(file: string, line?: number): Promise<void> {
    if (window.flowpilot?.openInIde) {
      await window.flowpilot.openInIde(file, line);
      return;
    }
    // eslint-disable-next-line no-console
    console.log("[IdeBridge stub:renderer] open in IDE:", { file, line });
  },

  async openExternal(url: string): Promise<void> {
    if (window.flowpilot?.openExternal) {
      await window.flowpilot.openExternal(url);
      return;
    }
    // Browser fallback (renderer running in a plain tab during dev).
    window.open(url, "_blank", "noopener");
  },
};
