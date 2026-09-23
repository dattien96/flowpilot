# CA-924 — Desktop embedded terminal, VS Code-style (CP-83 / Task-426..428)

## Context

CP-83 adds a real shell inside the desktop app — VS Code-style bottom dock —
scoped strictly outside FlowPilot core: no run/timeline/store coupling, no
audit-trail writes. Operator gets a shell pinned to the focused run's
effective directory (worktree when bound, else project root).

## Changes

### Task-426 — worktreePath contract (runner → UI)

- `RunHistoryItem.WorktreePath` + `RunSnapshot.WorktreePath` (Go) populated
  from `ProviderSessionState.WorktreePath` / run binding at start, resume,
  and history listing (resident + persisted paths).
- TS `contract.ts` mirrors: `worktreePath?: string` on both interfaces —
  additive optional, no breaking change.

### Task-427 — PTY manager + preload bridge

- `electron/terminal.ts` `registerTerminalIpc(getWindow)`: `term:spawn` /
  `term:write` / `term:resize` / `term:kill` handlers + push channels
  `term:data` / `term:exit`. Lazy `require("node-pty")` so the module only
  loads on first spawn. Default shell: `$SHELL`, Windows `COMSPEC` →
  `powershell.exe`. cwd is validated (`fs.statSync` isDirectory) before
  spawn — a GC'd worktree rejects with a visible error, never silently opens
  elsewhere.
- `src/terminal/ptyRegistry.ts` — pure registry (spawn/write/resize/kill/
  killAll, id-keyed, spawn-failure leaves no entry) kept Electron-free for
  node:test.
- `src/terminal/termBridge.ts` — pure bridge factory mapping the four
  invokes + `onData`/`onExit` subscribe→unsubscribe onto an IPC-shaped
  adapter; preload wires it to `ipcRenderer`.
- `electron/preload.ts` exposes `flowpilot.term` (typed in
  `flowpilotBridge.d.ts`). `contextIsolation` stays on; no `child_process`/
  fs reaches the renderer.
- Cleanup: `before-quit` + `webContents` `destroyed` / `render-process-gone`
  / `did-start-navigation` all `killAll()` — reload/close never orphans
  shells.
- `main.ts` registers IPC in `whenReady` alongside lifecycle.

### Task-428 — panel UI

- `src/terminal/cwd.ts` `resolveTerminalCwd`: `activeWorktreePath` (pinned
  from `RunHandle`/`RunHistoryItem` at run open) → focused run's history
  `worktreePath` → `selectedProject.path` → null.
- `src/terminal/termTabs.ts` `TerminalTabs`: tab lifecycle (open pins cwd at
  spawn, activate, close→kill unless exited, markExited, write/resize
  passthrough, closeAll). Pure — no React/xterm.
- `src/components/TerminalPanel.tsx`: 240px dock inside `.workspace-main`
  (flex row under the chat column, not overlay), tab strip (cwd basename +
  `exited` marker + close), xterm + FitAddon per tab, ResizeObserver→fit→
  `term.resize`, spawn errors render in-panel (T-4).
- `App.tsx`: header `TerminalIcon` toggle + global `Ctrl+\``.
- `store.ts`: `terminalOpen`/`toggleTerminal()` + `activeWorktreePath` state
  (reset in `resetRun`, survives project switching per spec).
- `styles.css`: `.term-*` block, all spacing/radius from tokens;
  `styles.tokens.test.ts` emoji-guard list += `TerminalPanel.tsx`
  (additive coverage — strengthens the guard, weakens nothing).
- Deps: `node-pty@1.1.0-beta39`, `@xterm/xterm@6.0.0`,
  `@xterm/addon-fit@0.11.0` — all ≥7d old.

## Safe-fix compliance

- Zero existing tests edited. Guard-list addition is additive strengthening
  only; `worktreePath` fields optional end-to-end.
- New tests: `terminal.test.ts` (8: registry spawn/data/kill/destroy/cwd
  reject, bridge channel map, subscribe/unsub) + `terminalPanel.test.ts`
  (11: cwd precedence×4, spawn pinning, spawn-failure hygiene, exit marking,
  close-kill, activation, write/resize passthrough, toggleTerminal).

## Live verification (this machine)

- `ELECTRON_RUN_AS_NODE` smoke: node-pty NAPI prebuild loads under Electron
  33's Node 20.18.3 — spawns `cmd.exe`, receives banner+prompt, kills clean.
  (`install-app-deps` node-gyp rebuild is unnecessary — NAPI prebuilds are
  ABI-stable; the earlier "no Visual Studio" failure is a non-blocker.)
- CDP live acceptance on the built app (`electron . --remote-debugging`):
  `flowpilot.term.spawn({cwd:"C:/",cols:80,rows:24})` → `{"id":"term-1"}`;
  `onData` streamed the real `Microsoft Windows` banner + `C:\>` prompt;
  `kill` → `{"ok":true}`. Full preload→IPC→pty→renderer chain verified.
- `vite build` green: `dist-electron/main.js` externalizes `node-pty`
  correctly (lazy require intact).

## Known edges (accepted)

- Shell env is `process.env` inherited from the app (PATH etc.) — per-task
  scope; profile picker is Task-428 out-of-scope.
- `Ctrl+\`` conflicts with Electron menu accelerators: none registered, and
  VS Code uses the same binding — verified no conflict on Windows.
- Hidden-panel shells keep running (VS Code parity); renderer reload kills
  all ptys (deliberate — prevents orphans; new window = fresh tabs).

## GitNexus

MCP unreachable — manual blast-radius: all new symbols; `store.ts` additions
are new optional fields/actions; `main.ts`/`preload.ts` additive call sites.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: CP-83
change_type: feature
summary: Embedded VS Code-style terminal — main-process node-pty registry + context-isolated flowpilot.term bridge + xterm bottom dock with cwd pinned to worktreePath-or-project; zero coupling to run/timeline state
# --->8---
