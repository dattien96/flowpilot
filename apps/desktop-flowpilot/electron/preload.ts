import { contextBridge, ipcRenderer } from "electron";

// Exposes a tiny, typed bridge to the renderer. Part A: openInIde is a stub on
// the main side (logs only). The renderer's IdeBridge falls back to console.log
// when this bridge is absent (e.g. running the renderer in a plain browser tab).
contextBridge.exposeInMainWorld("flowpilot", {
  openInIde: (file: string, line?: number): Promise<{ ok: boolean; stub?: boolean }> =>
    ipcRenderer.invoke("ide:open", { file, line }),
  openExternal: (url: string): Promise<{ ok: boolean }> => ipcRenderer.invoke("shell:openExternal", { url }),
  loadAuthSession: (): Promise<{
    clientKey: string;
    accessToken: string;
    refreshToken: string;
    userId: string;
    email?: string | null;
  } | null> => ipcRenderer.invoke("auth-session:load"),
  saveAuthSession: (payload: {
    clientKey: string;
    accessToken: string;
    refreshToken: string;
    userId: string;
    email?: string | null;
  }): Promise<{ ok: boolean }> => ipcRenderer.invoke("auth-session:save", payload),
  clearAuthSession: (): Promise<{ ok: boolean }> => ipcRenderer.invoke("auth-session:clear"),
});
