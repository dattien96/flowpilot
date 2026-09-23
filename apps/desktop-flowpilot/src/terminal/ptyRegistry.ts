// Task-427 (CP-83): pure terminal-pty registry — no Electron imports, so the
// lifecycle logic is unit-testable under plain node:test. The Electron adapter
// (electron/terminal.ts) only wires ipcMain channels and webContents.send.
//
// Hard isolation contract: this module knows cwd/shell/size only — no runId,
// no projectId, no FlowPilot store. Manual shell commands stay outside the
// agent audit trail by construction.

export interface TermSpawnRequest {
  cwd: string;
  shell?: string;
  cols: number;
  rows: number;
}

/** Structural subset of node-pty's IPty the registry depends on. */
export interface PtyLike {
  write(data: string): void;
  resize(cols: number, rows: number): void;
  kill(): void;
  onData(cb: (data: string) => void): void;
  onExit(cb: (e: { exitCode: number; signal?: number }) => void): void;
}

export type PtySpawner = (req: Required<Omit<TermSpawnRequest, "shell">> & { shell: string }) => PtyLike;

export interface TermEvents {
  onData(id: string, data: string): void;
  onExit(id: string, exitCode: number): void;
}

/** resolveShell: explicit request > $SHELL (POSIX) > platform default.
 *  Windows defaults to PowerShell (VS Code parity), falling back to COMSPEC
 *  when PowerShell is not installed. */
export function resolveShell(
  requested: string | undefined,
  env: NodeJS.ProcessEnv = process.env,
  platform: NodeJS.Platform = process.platform,
): string {
  const trimmed = requested?.trim();
  if (trimmed) return trimmed;
  const envShell = env.SHELL?.trim();
  if (envShell) return envShell;
  if (platform === "win32") {
    return env.COMSPEC?.trim() || "powershell.exe";
  }
  return "/bin/sh";
}

export class PtyRegistry {
  private readonly ptys = new Map<string, PtyLike>();
  private seq = 0;

  constructor(
    private readonly spawnPty: PtySpawner,
    private readonly events: TermEvents,
  ) {}

  get size(): number {
    return this.ptys.size;
  }

  has(id: string): boolean {
    return this.ptys.has(id);
  }

  spawn(req: TermSpawnRequest): { id: string } {
    const shell = resolveShell(req.shell);
    const cols = Math.max(1, Math.floor(req.cols) || 80);
    const rows = Math.max(1, Math.floor(req.rows) || 24);
    // Let the spawner throw (bad cwd / missing shell) BEFORE allocating an id —
    // a failed spawn leaves no registry entry (AC-6).
    const pty = this.spawnPty({ cwd: req.cwd, shell, cols, rows });
    const id = `term-${++this.seq}`;
    this.ptys.set(id, pty);
    pty.onData((data) => this.events.onData(id, data));
    pty.onExit(({ exitCode }) => {
      if (this.ptys.delete(id)) {
        this.events.onExit(id, exitCode);
      }
    });
    return { id };
  }

  write(id: string, data: string): void {
    this.ptys.get(id)?.write(data);
  }

  resize(id: string, cols: number, rows: number): void {
    const c = Math.floor(cols);
    const r = Math.floor(rows);
    if (!Number.isFinite(c) || !Number.isFinite(r) || c < 1 || r < 1) return;
    this.ptys.get(id)?.resize(c, r);
  }

  kill(id: string): { ok: boolean } {
    const pty = this.ptys.get(id);
    if (!pty) return { ok: false };
    this.ptys.delete(id);
    try {
      pty.kill();
      return { ok: true };
    } catch {
      return { ok: false };
    }
  }

  /** Kill every owned pty — app quit, window destroy, renderer reload. */
  killAll(): void {
    for (const [id, pty] of this.ptys) {
      this.ptys.delete(id);
      try {
        pty.kill();
      } catch {
        // best effort — the process is going away regardless
      }
    }
  }
}
