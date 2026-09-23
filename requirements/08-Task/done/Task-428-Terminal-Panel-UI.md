# Task-428 — Terminal Panel UI (VS Code-style Bottom Dock)

- Document ID: `Task-428`
- Title: `Terminal Panel UI`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: `CP-83 (Embedded Terminal)`
- Child Documents: ``
- Related Documents: `Task-426 (worktreePath), Task-427 (pty bridge)`
- Replaces: ``
- Tags: `terminal-session, desktop, chat-ui`

## AI Quick View

### Summary

- VS Code-style bottom panel: toggle from header/`Ctrl+\``, tab strip for
  multiple shells, `xterm.js` + `FitAddon` rendering, kill/close per tab.
- cwd resolution is the only FlowPilot-aware piece: focused run's
  `worktreePath` (Task-426) → else `selectedProject.path` — resolved at spawn
  time and pinned per tab.

### Current Ask

- Implement `TerminalPanel` + `terminalOpen` store flag + cwd resolver +
  styles; wire into `App.tsx`.

### Key Decisions

- `T-1` cwd pins at spawn: opening a terminal in a worktree-bound run lands in
  the tree; later binding changes don't retarget live shells (a "new
  terminal" picks up the new cwd — VS Code behavior).
- `T-2` Panel height persisted in store session (not localStorage — v1 fixed
  240px default, drag-resize optional follow-up).
- `T-3` Tabs are labeled by their cwd basename + index ("myproj (worktree)"
  when applicable); exit state marks the tab, close kills the pty.
- `T-4` If resolved cwd no longer exists on disk (GC'd worktree), spawn shows
  the IPC error in the terminal buffer — never silently opens elsewhere.

### Constraints

- Terminal is outside the run audit trail by design — no status/timeline
  writes; `styles.tokens.test.ts` passes with `TerminalPanel.tsx` guarded.
- `xterm` theme must consume CSS variables (background/foreground tokens),
  not hardcoded colors.

### Open Questions

- Keyboard shortcut conflict check on Windows (`Ctrl+`` vs browser/menu
  accelerators) — verify live.

### Source Refs

- `CP-83 P-4`, `Task-426`, `Task-427`

## 1. Goal

Operator opens a real shell rooted at the focused run's effective directory —
repo root normally, worktree when bound — inside the app window.

## 2. Parent Links

- coding plan: CP-83
- tech design: —
- system spec: SS-13
- specific upstream ids: Task-426, Task-427

## 3. Trigger

CP-83 P-4 — the visible half of the embedded terminal.

## 4. Exact Change

- `T-1` `store.ts`: `terminalOpen: boolean`, `toggleTerminal()`, plus
  `terminalTabs` local state inside the component (not store — UI-local).
- `T-2` `src/terminal/cwd.ts` (new): `resolveTerminalCwd(state)`.
- `T-3` `src/components/TerminalPanel.tsx`: tab strip + xterm mount +
  spawn/write/resize/kill wiring through `flowpilot.term`.
- `T-4` `App.tsx`: header Terminal icon toggle + panel mount under chat
  column (grid row, not overlay — pushes timeline like VS Code).
- `T-5` `styles.css`: `.term-panel`, `.term-tabs`, `.term-tab*` —
  token-based; `styles.tokens.test.ts` guard list += `TerminalPanel.tsx`.

## 5. Touched Areas

- files: `src/terminal/cwd.ts` (new), `src/components/TerminalPanel.tsx`
  (new), `src/state/store.ts`, `src/App.tsx`, `src/styles.css`,
  `src/styles.tokens.test.ts`, `src/components/icons.tsx` (TerminalIcon)
- modules: desktop renderer
- routes: none
- tables: none

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/terminal/cwd.ts
/** Focused run's worktreePath (when bound) else the selected project path. */
export function resolveTerminalCwd(state: {
  runId: string | null;
  runHistory: RunHistoryItem[];      // focused project slice
  selectedProjectId: string | null;
  projects: ProjectInfo[];
}): string | null
```

```ts
// apps/desktop-flowpilot/src/state/store.ts
interface AppState {
  terminalOpen: boolean;
  toggleTerminal(): void;
}
```

```tsx
// apps/desktop-flowpilot/src/components/TerminalPanel.tsx
interface TermTab { id: string; title: string; exited: boolean; }
export function TerminalPanel(): JSX.Element | null
// mount: new Terminal({theme from tokens}) + FitAddon; spawn via
// flowpilot.term.spawn({cwd: resolveTerminalCwd(store), cols, rows});
// xterm.onData → term.write; term.onData → xterm.write; onExit → tab.exited
```

## 7. Test Signatures

- `test("resolveTerminalCwd returns worktreePath for bound focused run", ...)` — T-1/AC-1
- `test("resolveTerminalCwd falls back to project.path when unbound", ...)` — AC-2
- `test("resolveTerminalCwd returns null with no project selected", ...)` — AC-3
- `test("toggleTerminal flips terminalOpen", ...)` — AC-4
- `test("panel spawns pty with resolved cwd on new tab", ...)` — bridge spy (AC-5)
- `test("exited pty marks tab; close kills via bridge", ...)` — AC-6
- `test("terminal never calls run/timeline store actions", ...)` — zero-coupling invariant (AC-7)

## 8. Acceptance Check

- In a worktree-bound run: new terminal's `cd`/`pwd` shows
  `.flowpilot/worktrees/<owner>`; unbound run → project root.
- `Ctrl+\`` toggles; panel survives project switching; closing the app leaves
  no shell processes.
- Panel at 960px window: tabs truncate, no horizontal overflow.

## 9. Out of Scope

- Split terminals, link detection, search addon, persistent scrollback,
  shell profile picker, drag-resize (v1 fixed height).

## 10. Definition of Done

- [ ] §6 signatures implemented (or §11 deviation)
- [ ] §7 tests green, additive-only; old suite green (R1)
- [ ] Provider-agnostic evidenced (R2)
- [ ] `feature_key` = `terminal-session`; CA entry
- [ ] §8 verified live on this machine; `detect_changes` clean

## 11. Completion Notes

- result: implemented — resolveTerminalCwd (worktreePath→project.path), TerminalTabs lifecycle, TerminalPanel dock (tabs+exit mark+close-kill), header toggle + Ctrl+`, token styles; 11 new tests green; CDP live test passed (spawn/data/kill real cmd.exe).
- follow-ups: drag-resize, split, search addon remain out-of-scope follow-ups; hidden-panel shells persist by design.
- upstream docs updated: CA-924.
