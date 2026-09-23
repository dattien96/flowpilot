export {};

// CP-81: renderer-facing lifecycle bridge (SD-28 §6.3). Token material never
// crosses this boundary — fenced actions execute inside Electron main.
interface RunnerLifecycleStatusEvent {
  connected: boolean;
  phase?: string;
  sharedClients: number;
  activeWork: number;
  idleDeadlineMs?: number;
  updatePending: boolean;
  reconnecting: boolean;
}

interface RunnerLifecycleActionResult {
  ok: boolean;
  code?: string;
  restartId?: string;
  confirmationRequired?: boolean;
}

interface RunnerLifecycleBridge {
  getSnapshot(): Promise<unknown>;
  requestClose(): Promise<"cancelled" | "closed">;
  requestGlobalShutdown(): Promise<RunnerLifecycleActionResult>;
  requestRestart(): Promise<RunnerLifecycleActionResult>;
  onStatus(cb: (status: RunnerLifecycleStatusEvent) => void): () => void;
}

declare global {
  interface Window {
    flowpilot?: {
      platform?: NodeJS.Platform | string;
      openInIde(file: string, line?: number): Promise<{ ok: boolean; stub?: boolean }>;
      openExternal(url: string): Promise<{ ok: boolean }>;
      loadAuthSession(): Promise<{
        clientKey: string;
        accessToken: string;
        refreshToken: string;
        userId: string;
        email?: string | null;
      } | null>;
      saveAuthSession(payload: {
        clientKey: string;
        accessToken: string;
        refreshToken: string;
        userId: string;
        email?: string | null;
      }): Promise<{ ok: boolean }>;
      clearAuthSession(): Promise<{ ok: boolean }>;
      requestHttp(payload: {
        url: string;
        method?: string;
        headers?: Record<string, string>;
        body?: string;
      }): Promise<{
        status: number;
        headers: Array<[string, string]>;
        body: string;
      }>;
      showNotification(title: string, body: string): Promise<{ ok: boolean }>;
      lifecycle?: RunnerLifecycleBridge;
    };
  }
}
