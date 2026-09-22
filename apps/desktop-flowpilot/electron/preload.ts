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
  requestHttp: (payload: {
    url: string;
    method?: string;
    headers?: Record<string, string>;
    body?: string;
  }): Promise<{
    status: number;
    headers: Array<[string, string]>;
    body: string;
  }> => ipcRenderer.invoke("http:request", payload),
  showNotification: (title: string, body: string): Promise<{ ok: boolean }> =>
    ipcRenderer.invoke("notification:show", { title, body }),
  isGitRepo: (path: string): Promise<boolean> => ipcRenderer.invoke("project:isGitRepo", { path }),
  // CP-81: lifecycle bridge — renderer sees snapshots/status but never the
  // lease token; destructive actions are fenced inside Electron main.
  lifecycle: {
    getSnapshot: (): Promise<unknown> => ipcRenderer.invoke("lifecycle:snapshot"),
    requestClose: (): Promise<"cancelled" | "closed"> => ipcRenderer.invoke("lifecycle:requestClose"),
    requestGlobalShutdown: (): Promise<unknown> => ipcRenderer.invoke("lifecycle:shutdown"),
    requestRestart: (): Promise<unknown> => ipcRenderer.invoke("lifecycle:restart"),
    onStatus: (cb: (status: unknown) => void): (() => void) => {
      const listener = (_e: unknown, status: unknown): void => cb(status);
      ipcRenderer.on("lifecycle:status", listener);
      return () => ipcRenderer.removeListener("lifecycle:status", listener);
    },
  },
});
