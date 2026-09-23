import type { TermSpawnRequest } from "./termBridge";

/** Minimal slice of the preload term bridge the panel needs. */
export interface TermBridgeLike {
  spawn(req: TermSpawnRequest): Promise<{ id: string }>;
  write(id: string, data: string): Promise<void>;
  resize(id: string, cols: number, rows: number): Promise<void>;
  kill(id: string): Promise<{ ok: boolean }>;
}

export interface TermTabState {
  id: string;
  title: string;
  cwd: string;
  exited: boolean;
}

function cwdBasename(cwd: string): string {
  const trimmed = cwd.replace(/[\\/]+$/, "");
  const base = trimmed.split(/[\\/]/).pop() ?? "";
  return base || cwd;
}

/**
 * Task-428: tab lifecycle for the terminal dock — spawn/close/mark-exited via
 * the bridge, cwd pinned per tab at spawn time (T-1). No React, no xterm, no
 * run-state coupling: the component injects resolveCwd and owns rendering.
 */
export class TerminalTabs {
  readonly tabs: TermTabState[] = [];
  activeId: string | null = null;

  constructor(
    private readonly bridge: TermBridgeLike,
    private readonly resolveCwd: () => string | null,
    private readonly notify: () => void,
  ) {}

  get active(): TermTabState | undefined {
    return this.tabs.find((t) => t.id === this.activeId);
  }

  /** Spawns a shell pinned to the cwd resolved NOW (T-1). Returns tab id. */
  async open(cols: number, rows: number): Promise<string> {
    const cwd = this.resolveCwd();
    if (!cwd) throw new Error("no project selected — pick a project first");
    const { id } = await this.bridge.spawn({ cwd, cols, rows });
    this.tabs.push({ id, title: cwdBasename(cwd), cwd, exited: false });
    this.activeId = id;
    this.notify();
    return id;
  }

  activate(id: string): void {
    if (!this.tabs.some((t) => t.id === id)) return;
    this.activeId = id;
    this.notify();
  }

  async close(id: string): Promise<void> {
    const idx = this.tabs.findIndex((t) => t.id === id);
    if (idx < 0) return;
    const tab = this.tabs[idx];
    if (!tab.exited) {
      try {
        await this.bridge.kill(id);
      } catch {
        // Shell may already be gone — close the tab regardless.
      }
    }
    this.tabs.splice(idx, 1);
    if (this.activeId === id) {
      this.activeId = this.tabs[Math.min(idx, this.tabs.length - 1)]?.id ?? null;
    }
    this.notify();
  }

  markExited(id: string): void {
    const tab = this.tabs.find((t) => t.id === id);
    if (!tab || tab.exited) return;
    tab.exited = true;
    this.notify();
  }

  write(id: string, data: string): void {
    void this.bridge.write(id, data);
  }

  resize(id: string, cols: number, rows: number): void {
    void this.bridge.resize(id, cols, rows).catch(() => {
      // Shell may have exited between render and resize — ignore.
    });
  }

  async closeAll(): Promise<void> {
    for (const tab of [...this.tabs]) {
      await this.close(tab.id);
    }
  }
}
