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

// Task-427 (CP-83): embedded terminal bridge — mirror of
// src/terminal/termBridge.ts TermApi (kept structurally identical so the
// preload factory stays the single implementation).
interface TermSpawnRequest {
  cwd: string;
  shell?: string;
  cols: number;
  rows: number;
}

interface TermBridge {
  spawn(req: TermSpawnRequest): Promise<{ id: string }>;
  write(id: string, data: string): Promise<void>;
  resize(id: string, cols: number, rows: number): Promise<void>;
  kill(id: string): Promise<{ ok: boolean }>;
  onData(cb: (e: { id: string; data: string }) => void): () => void;
  onExit(cb: (e: { id: string; exitCode: number }) => void): () => void;
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
      /** CP-83: absent when the renderer runs outside Electron (plain browser). */
      term?: TermBridge;
    };
  }
}
