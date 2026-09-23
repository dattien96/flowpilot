import { app, BrowserWindow, ipcMain } from "electron";
import { statSync } from "node:fs";
import path from "node:path";

import { PtyRegistry, type PtyLike, type TermSpawnRequest } from "../src/terminal/ptyRegistry";

// Task-427 (CP-83): Electron adapter for the embedded terminal. All logic lives
// in src/terminal/ptyRegistry.ts (unit-tested); this file only owns the
// node-pty spawner, IPC channels, and cleanup hooks.
//
// Isolation contract: the registry is keyed by opaque terminal ids and knows
// nothing about runs/projects — cwd arrives fully resolved from the renderer
// (worktreePath ?? project.path, Task-426/428).

type PtyModule = typeof import("node-pty");

let ptyModule: PtyModule | null = null;
function loadPty(): PtyModule {
  if (!ptyModule) {
    // Lazy require: node-pty is a native (NAPI) module — deferring keeps app
    // startup resilient if the prebuilt binary is absent, and keeps plain-node
    // unit tests free of the native load.
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    ptyModule = require("node-pty") as PtyModule;
  }
  return ptyModule;
}

function spawnRealPty(req: Required<Omit<TermSpawnRequest, "shell">> & { shell: string }): PtyLike {
  // Fail fast on a missing cwd so spawn rejects before allocating an id (AC-6).
  if (!statSync(req.cwd, { throwIfNoEntry: false })?.isDirectory()) {
    throw new Error(`terminal cwd does not exist: ${req.cwd}`);
  }
  const pty = loadPty().spawn(req.shell, [], {
    name: "xterm-256color",
    cwd: path.resolve(req.cwd),
    cols: req.cols,
    rows: req.rows,
    env: process.env as Record<string, string>,
    // ConPTY on modern Windows; node-pty falls back to winpty automatically.
    useConpty: process.platform === "win32",
  });
  return pty;
}

/** Registers the term:* IPC surface against the single app window. Idempotent
 *  per app lifetime — safe to call once during app ready. */
export function registerTerminalIpc(getWindow: () => BrowserWindow | null): PtyRegistry {
  const send = (channel: string, payload: unknown): void => {
    const win = getWindow();
    if (win && !win.isDestroyed()) win.webContents.send(channel, payload);
  };
  const registry = new PtyRegistry(spawnRealPty, {
    onData: (id, data) => send("term:data", { id, data }),
    onExit: (id, exitCode) => send("term:exit", { id, exitCode }),
  });

  ipcMain.handle("term:spawn", (_e, req: TermSpawnRequest) => registry.spawn(req));
  ipcMain.handle("term:write", (_e, p: { id: string; data: string }) => registry.write(p.id, p.data));
  ipcMain.handle("term:resize", (_e, p: { id: string; cols: number; rows: number }) =>
    registry.resize(p.id, p.cols, p.rows),
  );
  ipcMain.handle("term:kill", (_e, p: { id: string }) => registry.kill(p.id));

  // T-3 lifecycle: no orphaned shells. before-quit covers app exit; the
  // webContents hooks cover window close, renderer crash, and reload (the
  // renderer's pty handles die with its JS context — the OS processes must too).
  app.on("before-quit", () => registry.killAll());
  const hookWindow = (win: BrowserWindow | null): void => {
    win?.webContents.on("destroyed", () => registry.killAll());
    win?.webContents.on("render-process-gone", () => registry.killAll());
    // did-start-navigation fires on loadURL/reload — kill stragglers before a
    // fresh renderer ever spawns new terminals.
    win?.webContents.on("did-start-navigation", () => registry.killAll());
  };
  app.on("browser-window-created", (_e, win) => hookWindow(win));
  hookWindow(getWindow());

  return registry;
}
