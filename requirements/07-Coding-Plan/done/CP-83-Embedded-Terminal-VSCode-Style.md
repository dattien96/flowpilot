# CP-83 — Embedded Terminal (VS Code-style) for Desktop App

- Document ID: `CP-83`
- Title: `Embedded Terminal Panel`
- Phase: `coding_plan`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `Task-405 (desktop UI), SS-23/SD-27 (worktree isolation), CA-922`
- Child Documents: `Task-426 (worktreePath), Task-427 (pty+bridge), Task-428 (panel UI), CP-83-Test-Steps`
- Related Documents: `CP-82 (multi-project ops), CP-71 (worktree isolation)`
- Replaces: ``
- Tags: `desktop, terminal, electron, worktree`

## AI Quick View

### Summary

- Add a VS Code-style integrated terminal to the FlowPilot desktop app:
  `xterm.js` renders in the renderer, `node-pty` spawns a real shell in the
  Electron **main** process, IPC via the existing preload bridge.
- The terminal is a dumb shell — zero coupling to Core FlowPilot: no store
  run state, no runner API, no timeline, no ledger. It is an operator tool
  that happens to live in the same window.
- `cwd` resolution is the only place it touches FlowPilot semantics: terminal
  opens at the run's **worktree path** when the focused run has a worktree
  binding, otherwise at `project.path`.

### Current Ask

- A bottom-panel terminal (VS Code layout) that opens a real interactive
  shell in the correct project/worktree directory, with multi-tab support,
  and provably zero side-effects on run/orchestration state.

### Key Decisions

- `P-1` `node-pty` in Electron main + `xterm.js` in renderer (same stack as
  VS Code). IPC: preload exposes a typed `term.*` bridge; renderer never
  holds a process handle.
- `P-2` cwd resolution order: focused run's worktree path → `project.path`.
  The worktree path must come from the runner, not be computed client-side —
  add `worktreePath` to the run snapshot/history contract (the data already
  exists in `workflow_store.go`).
- `P-3` Panel placement: bottom panel toggled by a header icon + keyboard
  shortcut, matching VS Code muscle memory. Multiple terminals via tabs.
- `P-4` The terminal never auto-runs anything on behalf of a run and never
  writes to run state. Manual typing is outside the run audit trail — this is
  an accepted caveat, documented in UI copy if needed.

### Constraints

- `node-pty` is a native module — must be rebuilt for the Electron ABI;
  `electron-builder` handles this via `npmRebuild`. If it proves fragile on
  Windows, fallback is `child_process.spawn` + piped stdio (loses true-PTY
  programs like vim/htop but keeps shells usable).
- No changes to runner or Core FlowPilot logic — if a change seems needed, it
  is out of scope and must be flagged.

### Open Questions

- Default shell per platform: respect `$SHELL`/`COMSPEC`, or a settings
  option? (VS Code resolves a default profile list — start with OS default +
  one settings override.)
- Should the terminal cwd *follow* worktree binding changes mid-run, or pin
  at spawn time? (Recommend: pin at spawn; a "cd to worktree" button on the
  tab covers the change case.)

### Source Refs

- `Task-405`, `SS-23`/`SD-27` (D-7/D-8 worktree rules), `CA-922`, `CP-71`

## 1. Goal

Give the operator a real shell inside the app — to inspect files, run tests,
tail logs, or git-operate in the exact directory the agent is working in —
without leaving FlowPilot and without the terminal ever influencing a run.

## 2. Input Documents

- `SS-23` / `SD-27` — worktree path ownership (`<repo>/.flowpilot/worktrees/<ownerID>`,
  ownerID = chatId for chat runs, runId for flow runs) drives cwd resolution.
- `CA-922` — current desktop shell layout the panel must fit.
- `Task-405` — design tokens the panel must consume.

## 3. Implementation Strategy

- overall approach: three-layer sandwich, each layer independently testable —
  (a) main-process pty manager owning `pty.spawn` lifecycle keyed by
  terminalId; (b) preload bridge exposing `term.spawn/write/resize/kill` +
  `term.onData`; (c) renderer panel mounting `xterm.js` + `FitAddon`.
- sequencing logic: P-1 contract field first (unblocks correct cwd), then
  main+preload plumbing, then UI panel last so it lands already-wired.
- dependencies: `worktreePath` contract field (runner additive change); npm
  deps `node-pty`, `@xterm/xterm`, `@xterm/addon-fit`.

## 4. Work Breakdown

- `P-1` **Contract: expose `worktreePath`** — surface the already-persisted
  `WorktreePath` (`workflow_store.go`) on `RunSnapshot`/`RunHistoryItem` so
  the UI never reconstructs runner-internal paths. Additive field; no schema
  migration.
- `P-2` **Main-process pty manager** — `electron/terminal.ts`: map
  `terminalId → ptyProcess`; `spawn({cwd, shell})`, `write`, `resize`,
  `kill`; emits `term:data`/`term:exit` over `webContents.send`. Handles
  renderer reload (kill orphans), app quit (kill all).
- `P-3` **Preload bridge** — typed `window.flowpilot.term` API wrapping the
  IPC channels; mirrors existing bridge conventions in `preload.ts` +
  `flowpilotBridge.d.ts`.
- `P-4` **Renderer terminal panel** — bottom-docked panel (VS Code style):
  tab bar, `+` new terminal (cwd = resolved path), kill/close per tab,
  `FitAddon` resize sync, theme from design tokens. cwd resolution helper:
  `terminalCwd = focusedRun.worktreePath ?? selectedProject.path`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/electron/terminal.ts` (new),
  `electron/main.ts`, `electron/preload.ts`,
  `src/components/TerminalPanel.tsx` (new), `src/types/flowpilotBridge.d.ts`,
  `src/state/store.ts` (cwd resolver only), `src/styles.css`,
  `apps/local-runner` run snapshot/history mappers (`worktreePath` field)
- modules: desktop electron shell + renderer; one additive contract field
- database: none
- external systems: none

## 6. Data or Migration Steps

- schema: none (field is additive on an existing persisted struct).
- data backfill: none — runs without a worktree simply omit the field.
- config updates: `package.json` deps (`node-pty`, `@xterm/*`); builder
  config may need `npmRebuild`/`asarUnpack` for the native module.

## 7. Validation Plan

- tests to add:
  - `test("terminal cwd resolves to worktreePath when focused run has binding", …)`
  - `test("terminal cwd falls back to project.path without worktree", …)`
  - main-process: spawn/write/resize/kill lifecycle (node-pty mockable via
    seam, or gated integration test).
  - `test("closing app/run kills owned pty processes", …)`
- manual checks: open terminal in project A and worktree-backed run B —
  `pwd`/`cd` output shows correct distinct roots; typing in terminal never
  mutates timeline/status; interactive program (e.g. `npm test` watch)
  survives; window resize keeps terminal fitted.
- failure cases: worktree deleted on disk between binding and spawn → spawn
  falls back or errors visibly, never into a stale path; pty exits → tab
  shows exited state, close cleans up.

## 8. Rollout and Fallback

- rollout order: contract field → main/preload → panel. Each mergeable
  independently; panel hidden until wired.
- fallback path: remove the toggle — feature is fully additive.
- monitoring: none (local desktop app); renderer logs pty exit codes.

## 9. Risks

- `R-1` `node-pty` native build fragility on Windows/Electron ABI → fallback
  `child_process.spawn` (non-PTY) documented; decide after first build.
- `R-2` Orphaned shells if renderer reloads without cleanup → main owns
  lifecycle, kills on `webContents destroyed`.
- `R-3` User mistakes terminal for run console → label panel clearly;
  manual commands are outside the run ledger by design.
- `R-4` cwd computed client-side would drift from runner truth → P-1 contract
  field removes this class of bug.

## 10. Definition of Done

- [x] Bottom terminal panel opens/closes like VS Code, multi-tab. — Task-428
- [x] Shell spawns in `project.path`, or the run's `worktreePath` when bound. — resolveTerminalCwd; live-verified worktreePath contract
- [x] `worktreePath` exposed via contract; UI never computes the path. — Task-426, live-verified on start/history/resume
- [x] Zero writes to run/timeline/orchestration state from terminal code —
      verified by test + code review.
- [x] Pty lifecycle clean on tab close, window reload, app quit. — registry kill-all on lifecycle events; live spawn/kill verified, no orphans
- [x] Typecheck + build green; focused tests added; CA entry written. — CA-924
- [x] Commit format `[Feature][desktop-ui|chat-ui][ui|electron]` per repo — be99ae35
      convention.
