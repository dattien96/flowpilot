# BUG-240: Windows Dev-Stack Shutdown Orphans The Compiled Go Runner Binary

## Metadata

- Document ID: `BUG-240`
- Title: `Windows Dev-Stack Shutdown Orphans The Compiled Go Runner Binary`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: `none`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `dev-tooling, supervisor, windows, process-management, local-runner, regression`

## AI Quick View

### Summary

- On Windows, stopping the dev stack — Ctrl-C in the `npm run dev` terminal, or the desktop app's "Turn off system" control — leaves the compiled Go runner binary (`flowpilot.exe`, built by `go run` into `%TEMP%\go-build*\b001\exe\flowpilot.exe`) running as an orphan. macOS/Linux shut down cleanly via the same path.
- Root cause: `scripts/supervisor.js` tracks only the `go.exe`/`cmd.exe` wrapper PID for the runner (Windows has no POSIX `exec()`-replace, so `go run` always keeps the compiled binary as a distinct child process). Shutdown signaled only that wrapper PID, and the liveness poll gating the tree-kill fallback checked only that same wrapper PID — so once the wrapper exited (which happens quickly on Windows), the poll read "not alive" and exited immediately, never reaching the one code path (`killProcessTree`, `taskkill /F /T`) that actually tears down the whole process tree.
- Practical impact discovered live: the orphaned backend keeps serving on the old port with stale in-memory run state and an old build, so a code fix can appear "not to take effect" until someone manually finds and kills the leaked PID.
- Fixed by making `signalManagedProcess` go straight to the existing tree-kill (`taskkill /F /T /PID <pid>`) on Windows instead of a single-PID signal, for all three call sites that funnel through it (`stopManagedProcess`, `cleanupAndExit`, and the restart-flow cleanup). POSIX behavior (process-group `SIGINT` then escalate) is untouched.

### Current Ask

- Stop the compiled runner binary (and any other Windows-spawned managed process) from surviving every shutdown path: Ctrl-C, the UI "Turn off system" control, and process replacement during a restart.

### Key Decisions

- `D-1` Do not try to make `process.kill(pid, signal)` work correctly on Windows (it fundamentally can't reach a descendant PID, and Windows has no real graceful-signal semantics to preserve for a non-console-owning process). Instead, route Windows shutdown through the tree-aware `taskkill /F /T` that the codebase already had — just make it the immediate/primary mechanism on Windows rather than a 3-second-grace-period fallback gated behind a liveness check on the wrong PID.
- `D-2` Fix `signalManagedProcess` (the single function all three shutdown call sites share) rather than patching each call site separately, so `stopManagedProcess`, `cleanupAndExit`, and the restart-flow cleanup all inherit the fix uniformly.

### Constraints

- Dev-tooling only (`scripts/supervisor.js`); no application code, schema, or API surface changes. No dedicated automated test suite exists for this script — validated via manual process-tree reproduction (see `Validation`).
- POSIX (`detached: true` process-group signaling) must remain unchanged; only the Windows branch changes.

### Open Questions

- None.

### Source Refs

- `scripts/supervisor.js:489-511` — runner spawn: `go.exe`/`go run` with `shell:true`, `detached: process.platform !== 'win32'`.
- `scripts/supervisor.js:217-226` (pre-fix) — `signalManagedProcess`, single-PID `process.kill` on Windows.
- `scripts/supervisor.js:618-640` — `killProcessTree`, the pre-existing `taskkill /F /T /PID` implementation, previously only reached after a 3s grace period.
- `scripts/supervisor.js:653-692`, `694-740`, `310-327` — the three shutdown/restart call sites (`cleanupAndExit`, its restart-flow counterpart, `stopManagedProcess`) that all call `signalManagedProcess` then poll `isPidAlive` on the same tracked wrapper PID.
- Manual repro: spawned `go run ./cmd/flowpilot runner serve --port 8799` exactly as the supervisor does; confirmed via `Get-CimInstance Win32_Process` that the compiled `flowpilot.exe` is a child of the tracked `go.exe` PID; `process.kill(wrapperPid, 'SIGINT')` killed only the wrapper and left `flowpilot.exe` running; `taskkill /F /T /PID <wrapperPid>` killed the wrapper, the compiled binary, and one intermediate shell layer together.

## 1. Issue Summary

On Windows, every documented way of stopping FlowPilot's local dev stack (Ctrl-C in the terminal, or the desktop app's system-shutdown control) leaves the compiled Go runner server process running in the background, invisible to the user, holding its port and its last in-memory state. The same shutdown paths work correctly on macOS. The user only discovered this because a code fix appeared to have no effect — the desktop app was still talking to the orphaned pre-fix binary.

## 2. Parent Links

- impacted coding plan: none (dev-tooling script, not a shipped feature)
- impacted tech design: none
- impacted system spec: none

## 3. Environment and Reproduction

- environment: Windows 11, dev stack started via `npm run dev` (`scripts/supervisor.js`), which spawns the Go runner with `go run ./cmd/flowpilot runner serve --port <port>`.
- reproduction steps:
  1. Start the dev stack (or, minimally, spawn `go run ./cmd/flowpilot runner serve --port <port>` from `apps/local-runner` exactly as `scripts/supervisor.js` does: `shell:true`, `detached:false`).
  2. Wait for the compiled binary to appear as a child of the `go.exe` wrapper PID (confirmed via `Get-CimInstance Win32_Process -Filter "ProcessId=<compiledPid>"` → `ParentProcessId` = the wrapper PID).
  3. Stop the stack via Ctrl-C, or trigger the UI "Turn off system" control (`POST /system/shutdown` → supervisor `cleanupAndExit`).
  4. Observe: the `go.exe`/`cmd.exe` wrapper process exits, but the compiled `flowpilot.exe` keeps running and keeps serving on its port.
- frequency: always, on Windows; never reproduced on macOS/Linux (POSIX process-group signaling covers the whole tree).

## 4. Expected vs Actual

- expected: stopping the dev stack terminates the runner wrapper and the compiled server binary together, on every platform.
- actual: on Windows, only the wrapper process is reliably terminated; the compiled server binary is orphaned and keeps running indefinitely.

## 5. Impact

- users affected: any Windows developer running the FlowPilot dev stack.
- workflows affected: local development iteration — an orphaned backend silently serves stale code/state, making a just-applied fix look like it "didn't work" until the leaked process is found and killed manually. No production impact (dev-tooling only).
- severity: medium — no data loss or corruption, but actively misleading during iteration and can accumulate multiple orphaned processes/ports across a session.

## 6. Root Cause

- hypothesis: Windows process-tree semantics differ from POSIX and the supervisor's kill logic assumed POSIX-style signal propagation.
- confirmed cause: two compounding gaps, both in `scripts/supervisor.js`:
  1. **Wrong assumption about what the tracked PID represents.** `go run` on Windows compiles to a temp binary and launches it as a genuinely separate child process — Windows has no `exec()`-replace semantics to fold the two together the way POSIX does. So `runnerProcess.pid` (from `spawn('go.exe', ['run', ...], { shell:true, detached:false })`, `scripts/supervisor.js:504-511`) is only ever the `go.exe`/`cmd.exe` wrapper, never the actual server process.
  2. **The tree-kill fallback was gated on the wrong liveness signal.** `signalManagedProcess` (pre-fix) called `process.kill(child.pid, signal)` on Windows — targeting only that wrapper PID, with no propagation to its descendant. The subsequent poll loops in `cleanupAndExit` (and its restart-flow counterpart) then checked `isPidAlive(runnerProcess.pid)` — the same wrapper PID — to decide whether to escalate to `killProcessTree` (the actual `taskkill /F /T /PID` tree-kill, already implemented at `scripts/supervisor.js:618-640`). Because the `go.exe`/`cmd.exe` wrapper exits quickly on Windows once it hands off to the compiled binary, the poll read "not alive" almost immediately and called `finishExit()` right away — the code path that runs `killProcessTree` only fires after a full 3-second grace period with the wrapper *still* alive, which is exactly the opposite of what actually happens. The tree-kill fallback that would have caught this was effectively unreachable in the common case.
- evidence: manual reproduction (see `Source Refs`) — spawning the runner exactly as the supervisor does, then applying the pre-fix single-PID `process.kill(wrapperPid, 'SIGINT')` left the compiled `flowpilot.exe` running after the wrapper died; applying `taskkill /F /T /PID <wrapperPid>` (the fix) while both were alive terminated the wrapper, the compiled binary, and an intermediate shell layer together, with nothing left behind.

## 7. Fix Strategy

- `F-1` In `signalManagedProcess` (`scripts/supervisor.js`), add a Windows branch that calls `killProcessTree(child)` (the existing `taskkill /F /T /PID <pid>` implementation) immediately instead of `process.kill(child.pid, signal)`. Windows has no meaningful graceful-vs-forceful distinction to preserve here, so there is no reason to defer the tree-kill behind a liveness poll.
- `F-2` Leave the POSIX branch (`detached && process.kill(-pid, signal)`, process-group signaling) unchanged — it already correctly reaches the whole tree.
- `F-3` No changes needed at the three call sites (`stopManagedProcess`, `cleanupAndExit`, restart-flow cleanup) — all three call `signalManagedProcess`, so they inherit the fix uniformly.

## 8. Validation

- `V-1` `node --check scripts/supervisor.js` — syntax clean.
- `V-2` Manual process-tree reproduction (no automated test suite exists for this dev-tooling script): spawned `go run ./cmd/flowpilot runner serve --port 8799` exactly as the supervisor does (`shell:true`, `detached:false`, `apps/local-runner` cwd). Confirmed the compiled `flowpilot.exe` is a real Windows child process of the tracked `go.exe` wrapper PID (`Get-CimInstance Win32_Process` → `ParentProcessId` match).
- `V-3` Confirmed the **pre-fix** behavior reproduces the bug: `process.kill(wrapperPid, 'SIGINT')` terminates only the wrapper; the compiled binary (and its port) survive.
- `V-4` Confirmed the **post-fix** behavior resolves it: `taskkill /F /T /PID <wrapperPid>` (what `signalManagedProcess` now calls immediately on Windows) terminates the wrapper, the compiled binary, and an intermediate shell layer together — `Get-Process` afterward shows none of them remaining.

## 9. Regression Guard

- tests: none automated (dev-tooling script outside the Go/TS application test suites); regression protection is the `V-2`–`V-4` manual reproduction recorded above plus the code comment at the fix site explaining why the Windows branch cannot use a plain signal.
- alerts: none applicable.
- audit checks: `change-audit/CA-240-windows-dev-stack-shutdown-tree-kill.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — dev-tooling infrastructure with no dedicated SS/SD/CP.
- notes left unchanged on purpose: `killProcessTree`'s POSIX branch and the 3-second grace-period polling loop shape are left as-is; only the Windows branch of `signalManagedProcess` changes.

## 11. Definition of Done

- `[x]` **DOD-1 — Fix implemented.** `signalManagedProcess` calls `killProcessTree` immediately on Windows instead of a single-PID signal.
- `[x]` **DOD-2 — POSIX behavior unchanged.** Process-group signaling branch untouched.
- `[x]` **DOD-3 — Manual validation.** Bug reproduced pre-fix and resolved post-fix via direct process-tree inspection (`V-2`–`V-4`).
- `[x]` **DOD-4 — Docs.** This doc in `done/`; `change-audit/CA-240-*` written.

## 12. Code Change Plan (per DoD item)

### DOD-1 / DOD-2 — `scripts/supervisor.js`
- `signalManagedProcess(child, signal)`: added a `process.platform === 'win32'` branch that calls `killProcessTree(child)` and returns, placed after the existing POSIX `detached` process-group branch and before the previous unconditional `process.kill(child.pid, signal)` (which now only runs for POSIX non-detached children). Comment documents why Windows can't use a plain signal (no `exec()`-replace, no real graceful-signal semantics) and cross-references this bug.
- No other functions changed — `stopManagedProcess`, `cleanupAndExit`, and the restart-flow cleanup all call `signalManagedProcess` and inherit the fix without their own changes.
