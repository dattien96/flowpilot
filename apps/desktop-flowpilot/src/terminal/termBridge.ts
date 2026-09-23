// Task-427 (CP-83): renderer-facing terminal bridge factory — inject the IPC
// primitives so the wiring is unit-testable without Electron. preload.ts calls
// this with the real ipcRenderer.

export interface TermSpawnRequest {
  cwd: string;
  shell?: string;
  cols: number;
  rows: number;
}

export interface TermApi {
  spawn(req: TermSpawnRequest): Promise<{ id: string }>;
  write(id: string, data: string): Promise<void>;
  resize(id: string, cols: number, rows: number): Promise<void>;
  kill(id: string): Promise<{ ok: boolean }>;
  /** Subscribe to terminal output; returns an unsubscribe fn. */
  onData(cb: (e: { id: string; data: string }) => void): () => void;
  /** Subscribe to process exit; returns an unsubscribe fn. */
  onExit(cb: (e: { id: string; exitCode: number }) => void): () => void;
}

type Invoke = (channel: string, payload?: unknown) => Promise<unknown>;
// `any[]` rest — must accept Electron's (event: IpcRendererEvent, ...args: any[])
// listener shape; never[] rejects the assignment.
type Listener = (event: unknown, ...args: any[]) => void;
type On = (channel: string, listener: Listener) => void;
type Off = (channel: string, listener: Listener) => void;

export function createTermApi(invoke: Invoke, on: On, off: Off): TermApi {
  return {
    spawn: (req) => invoke("term:spawn", req) as Promise<{ id: string }>,
    write: (id, data) => invoke("term:write", { id, data }) as Promise<void>,
    resize: (id, cols, rows) => invoke("term:resize", { id, cols, rows }) as Promise<void>,
    kill: (id) => invoke("term:kill", { id }) as Promise<{ ok: boolean }>,
    onData: (cb) => {
      const listener = (_e: unknown, ...args: unknown[]): void => cb(args[0] as { id: string; data: string });
      on("term:data", listener as Listener);
      return () => off("term:data", listener as Listener);
    },
    onExit: (cb) => {
      const listener = (_e: unknown, ...args: unknown[]): void => cb(args[0] as { id: string; exitCode: number });
      on("term:exit", listener as Listener);
      return () => off("term:exit", listener as Listener);
    },
  };
}
