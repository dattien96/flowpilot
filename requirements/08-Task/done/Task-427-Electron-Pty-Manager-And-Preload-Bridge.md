# Task-427 — Electron Pty Manager + Preload Bridge

- Document ID: `Task-427`
- Title: `Electron Pty Manager + Preload Terminal Bridge`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-83 (Embedded Terminal)`
- Child Documents: ``
- Related Documents: `Task-426 (worktreePath contract), Task-428 (panel UI)`
- Replaces: ``
- Tags: `terminal-session, electron, desktop`

## AI Quick View

### Summary

- Main-process pty manager (`electron/terminal.ts`) owning `node-pty`
  lifecycle keyed by terminalId; preload exposes a typed `flowpilot.term`
  bridge; events stream back over `webContents.send`.
- Zero FlowPilot coupling: the manager knows only `cwd`/`shell`/size — no
  runId, no projectId, no store.

### Current Ask

- Implement the main-process manager + IPC channels + preload bridge + bridge
  typings. Renderer panel lands in Task-428.

### Key Decisions

- `T-1` `node-pty` in main; fallback plan `child_process.spawn` documented if
  native build fails — decision recorded in §11 after first build.
- `T-2` Channels: `term:spawn` (invoke→{id}), `term:write`, `term:resize`,
  `term:kill`; events `term:data`/`term:exit` pushed via `webContents.send`.
- `T-3` Lifecycle ownership: main kills all ptys on `before-quit` and on
  `webContents destroyed` (renderer reload must not orphan shells).
- `T-4` Default shell: `process.env.SHELL` on POSIX; `COMSPEC`→
  `powershell.exe` on Windows (settings override deferred).

### Constraints

- `node-pty` needs Electron-ABI rebuild — `electron-builder` `npmRebuild`
  (already default); may require `asarUnpack`.
- Bridge API is the ONLY renderer access — no direct `child_process`/`fs`
  exposure; keep `contextIsolation` on, no new `nodeIntegration`.

### Open Questions

- ConPTY availability on this Windows machine vs winpty fallback (node-pty
  chooses automatically — verify live).

### Source Refs

- `CP-83 P-2/P-3`

## 1. Goal

A spawnable, writable, resizable, killable real shell owned by the main
process, reachable from the renderer through a minimal typed bridge.

## 2. Parent Links

- coding plan: CP-83
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-426

## 3. Trigger

CP-83: VS Code-style embedded terminal with hard isolation from Core
FlowPilot.

## 4. Exact Change

- `T-1` New `apps/desktop-flowpilot/electron/terminal.ts`: pty registry,
  spawn/write/resize/kill, event forwarding, cleanup hooks.
- `T-2` `electron/main.ts`: `registerTerminalIpc()` during app ready; kill
  hook in `before-quit` + window `closed`.
- `T-3` `electron/preload.ts`: `term` namespace on the `flowpilot` bridge.
- `T-4` `src/types/flowpilotBridge.d.ts`: `TermApi` typings.
- `T-5` `package.json`: deps `node-pty`, `@xterm/xterm`, `@xterm/addon-fit`
  (xterm consumed in Task-428; add together to pin once).

## 5. Touched Areas

- files: `electron/terminal.ts` (new), `electron/main.ts`,
  `electron/preload.ts`, `src/types/flowpilotBridge.d.ts`,
  `apps/desktop-flowpilot/package.json`
- modules: electron main + preload
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/electron/terminal.ts
export interface TermSpawnRequest { cwd: string; shell?: string; cols: number; rows: number; }
export function registerTerminalIpc(getWindow: () => BrowserWindow | null): void;
// internals: Map<string, IPty>; resolveShell(): string; killAll(): void;
// ipcMain.handle("term:spawn", (_e, req: TermSpawnRequest) => Promise<{ id: string }>)
// ipcMain.handle("term:write",  (_e, p: { id: string; data: string }) => void)
// ipcMain.handle("term:resize", (_e, p: { id: string; cols: number; rows: number }) => void)
// ipcMain.handle("term:kill",   (_e, p: { id: string }) => Promise<{ ok: boolean }>)
// push: win.webContents.send("term:data", { id, data }) / ("term:exit", { id, exitCode })
```

```ts
// apps/desktop-flowpilot/electron/preload.ts (inside exposeInMainWorld)
term: {
  spawn: (req: TermSpawnRequest) => Promise<{ id: string }>;
  write: (id: string, data: string) => Promise<void>;
  resize: (id: string, cols: number, rows: number) => Promise<void>;
  kill: (id: string) => Promise<{ ok: boolean }>;
  onData: (cb: (e: { id: string; data: string }) => void) => () => void;
  onExit: (cb: (e: { id: string; exitCode: number }) => void) => () => void;
}
```

## 7. Test Signatures

- `test("term bridge methods invoke the correct IPC channels", ...)` — preload spy (AC-1)
- `test("onData/onExit subscribe + unsubscribe cleanly", ...)` — AC-2
- main-process (node:test or vitest w/ mocked node-pty):
  `test("spawn registers pty and forwards data events", ...)` — AC-3
  `test("kill removes pty and emits exit", ...)` — AC-4
  `test("window destroy kills all owned ptys", ...)` — T-3/AC-5
  `test("spawn failure (bad cwd) rejects with error, no registry entry", ...)` — AC-6

## 8. Acceptance Check

- Live: `npm run dev` → devtools `await flowpilot.term.spawn({cwd: "C:/", cols: 80, rows: 24})`
  returns an id; `onData` receives prompt bytes; `kill` ends it; no orphan
  process after window reload.

## 9. Out of Scope

- Any renderer UI, cwd resolution from runs (Task-428), multiple windows,
  shell profile list, settings integration.

## 10. Definition of Done

- [ ] §6 signatures implemented; channels + bridge typed end-to-end
- [ ] §7 tests green, additive-only; existing electron/preload tests green (R1)
- [ ] Provider-agnostic (no provider surface) — evidenced (R2)
- [ ] `feature_key` = `terminal-session`; CA entry
- [ ] §8 live check verified; `detect_changes` clean

## 11. Completion Notes

- result: implemented — electron/terminal.ts registerTerminalIpc (4 channels + 2 push), pure ptyRegistry/termBridge, preload flowpilot.term, typed globals, killAll on quit/destroy/navigation. 8 new tests green.
- follow-ups: node-pty uses shipped NAPI prebuilds (electron-builder rebuild unnecessary; verified live under Electron 33).
- upstream docs updated: CA-924.
