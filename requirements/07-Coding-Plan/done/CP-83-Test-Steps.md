# CP-83 Test Steps — Embedded Terminal (VS Code-style)

## Metadata

- Document ID: `CP-83-TEST-STEPS`
- Title: `CP-83 Verification Steps (Automated + Manual + Live)`
- Phase: `verification`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-02-14`
- Last Updated: `2026-02-14`
- Parent Documents: [CP-83: Embedded Terminal](./CP-83-Embedded-Terminal-VSCode-Style.md)
- Child Documents: `None`
- Related Documents: [Task-426](../../08-Task/todo/Task-426-Expose-WorktreePath-In-Run-Contract.md), [Task-427](../../08-Task/todo/Task-427-Electron-Pty-Manager-And-Preload-Bridge.md), [Task-428](../../08-Task/todo/Task-428-Terminal-Panel-UI.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `terminal, electron, node-pty, xterm, test-steps, verification, cp-83`
- Feature Keys: `terminal-session, run-worktree`

## AI Quick View

### Summary

- Verification for: `worktreePath` contract field (Task-426), main-process
  pty manager + preload bridge (Task-427), bottom-dock terminal panel
  (Task-428).
- Live bed: this Windows machine — real Electron app + real shells, plus
  runner HTTP calls for the worktree-path contract.

### Current Ask

- Automated suites green → devtools-driven bridge checks → full UI
  verification incl. worktree cwd and orphan-process checks.

### Key Decisions

- `V-1` First build decides `node-pty` viability on Windows/Electron ABI;
  failure → documented `child_process` fallback (record in Task-427 §11).
- `V-2` cwd truth comes from the runner contract, never client-computed.

### Constraints

- Additive tests only; no old-test edits.
- Terminal must show zero side-effects on run state — verified explicitly.

---

## 1. Goal

A real interactive shell inside the app, rooted at the correct directory
(project root or bound worktree), with clean lifecycle and zero Core
FlowPilot coupling.

---

## 2. Automated — run first

```bash
# 1. Contract field (Task-426) — Go
cd apps/local-runner
go test ./internal/runner -run 'TestRunHistoryItem_IncludesWorktreePath|TestRunHistoryItem_OmitsWorktreePath' -count=1 -v

# 2. Pty manager + preload bridge (Task-427)
# Desktop tests run on compiled output — NOT vitest:
cd ../desktop-flowpilot
../../node_modules/.bin/tsc -p ../../tsconfig.phase1-tests.json   # or: npm run test:phase1
node --require ../../scripts/phase1-runtime.js --test ../../.phase1-tests/apps/desktop-flowpilot/src/terminal/terminal.test.js

# 3. Panel + cwd resolver (Task-428)
node --require ../../scripts/phase1-runtime.js --test ../../.phase1-tests/apps/desktop-flowpilot/src/terminal/terminalPanel.test.js

# 4. Regression gates
npx tsc --noEmit && npx vite build
node --require ../../scripts/phase1-runtime.js --test ../../.phase1-tests/apps/desktop-flowpilot/src/styles.tokens.test.js
```

| Step | Check | Pass when | Tick |
|---|---|---|---|
| 2.1 | Contract | worktreePath emitted/omitted correctly | [x] PASS 2026-09-23 (task426 tests + live: start/history/resume all emit it) |
| 2.2 | Pty manager | spawn/write/resize/kill/exit + cleanup tests green | [x] PASS 2026-09-23 (terminal registry+bridge 8/8) |
| 2.3 | Bridge | channel names, onData/onExit subscribe/unsub | [x] PASS 2026-09-23 |
| 2.4 | cwd resolver | worktree→project→null matrix green | [x] PASS 2026-09-23 |
| 2.5 | Panel | spawn-args, tab exit/close, zero-coupling spy | [x] PASS 2026-09-23 (11/11) |
| 2.6 | Regression | typecheck + build + tokens test green | [x] PASS 2026-09-23 |

---

## 3. Manual prep

| # | Item | How | Tick |
|---|---|---|---|
| P1 | node-pty built | `npm i` in app dir; native module loads (no ABI error in devtools console) | [x] PASS — NAPI prebuild verified under Electron runtime (Node 20.18.3) |
| P2 | App live | `npm run dev` window opens | [ ] |
| P3 | Scratch project | git repo project registered | [ ] |
| P4 | Worktree run | one run with worktree toggle ON completed/started | [ ] |

---

## 4. Manual / Devtools Verification

### Scenario M-1: Bridge smoke test (no UI needed)

```js
// devtools console
const t = await flowpilot.term.spawn({cwd: "C:/", cols: 80, rows: 24});
flowpilot.term.onData(e => console.log("DATA", e.id, e.data));
await flowpilot.term.write(t.id, "echo hello\n");
await flowpilot.term.kill(t.id);
```

Pass: `{id}` returned; `DATA` chunks arrive incl. `hello`; kill → exit event.

### Scenario M-2: Panel opens shell in project root

1. Select a normal (non-worktree) project/run → `Ctrl+\`` or header icon.
2. **Observe**: bottom panel, prompt at `<project.path>`; type `pwd`/`cd`.

### Scenario M-3: Worktree cwd

1. Focus a run with worktree binding → new terminal.
2. **Observe**: prompt inside `.flowpilot/worktrees/<ownerId>`; `git branch`
   shows `fp/<slug>-<suffix>`.

### Scenario M-4: Multi-tab + lifecycle

1. `+` second tab → new shell; switching tabs independent buffers.
2. `exit` in a shell → tab marked exited; `×` closes tab (pty killed).
3. Reload window / quit app → **no orphan shells**:
   `Get-Process powershell,node-pty -ErrorAction SilentlyContinue` before/after.

### Scenario M-5: Zero core coupling

1. With a run streaming, open terminal, run commands, close panel.
2. **Observe**: timeline/status/run state unchanged; runner logs show no
   run-related calls triggered by terminal activity.

### Scenario M-6: Deleted worktree fallback

1. Delete a bound worktree dir on disk → open terminal on that run.
2. **Observe**: visible error in terminal buffer or safe behavior — never a
   silent cd into a wrong/stale path.

### Scenario M-7: Narrow window

1. 960px width with panel open → tabs truncate, xterm reflows, no horizontal
   page scroll.

---

## 5. Live REAL Tests (runner HTTP, this machine)

```powershell
# L-1: worktreePath in contract
#   GET /client/projects/{id}/runs → bound run item has "worktreePath"
#   matching an existing .flowpilot/worktrees/* dir; unbound runs omit it.
#
# L-2: worktreePath absolute + correct owner
#   Compare item.worktreePath vs dir listing of <repo>/.flowpilot/worktrees/.
```

Pass: field present only on bound runs; path exists on disk.

---

## 6. Log Grep (audit evidence)

```text
term:spawn
term:exit
worktree_path
```

---

## 7. CP-83 Verification Complete When

- [x] §2 automated all green; old suite untouched & green.
- [~] M-1 PASS live via Electron CDP (flowpilot.term bridge → spawn → real
      cmd.exe prompt bytes → kill; no orphans). M-2..M-7 pending operator UI pass.
- [x] L-1/L-2 verified live: run-257829 returned worktreePath on start +
      history + resume; path matched on-disk .flowpilot/worktrees/<chatId>;
      unbound runs omit the field.
- [x] node-pty viability recorded: NAPI prebuild loads under Electron runtime,
      no rebuild needed.
- [x] CA entries per task written. — CA-924
