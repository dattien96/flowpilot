# BUG-065: Desktop "Open In IDE" Fails With spawn EINVAL On Windows

## Metadata

- Document ID: `BUG-065`
- Title: `Desktop Open In IDE Fails With spawn EINVAL On Windows`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [04-01 Phase 1 Desktop Mock MVP](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md)
- Child Documents: `none`
- Related Documents: [03 Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- Replaces: `none`
- Tags: `desktop, electron, ide-bridge, windows, cross-platform, regression`

## AI Quick View

### Summary

- Clicking "open in IDE" on a generated/changed file throws `UnhandledPromiseRejectionWarning: Error: spawn EINVAL` in the Electron main process and nothing opens.
- Root cause is CVE-2024-27980: since Node 18.20 / 20.12, spawning a `.cmd`/`.bat` without a shell throws `EINVAL`. On Windows the IDE launchers (`code`, `cursor`, `studio`) are `.cmd` shims, so every `execFile` candidate fails.
- On macOS the IDE CLIs (`code`, `cursor`) are an opt-in PATH install, so when none are present the action silently did nothing.

### Current Ask

- Done. On Windows, IDE launchers are spawned through a shell (with manual arg quoting) so the `.cmd` shims resolve. A synchronous-throw guard prevents an unhandled rejection. An OS-default `shell.openPath` fallback guarantees the file still opens on win32/macOS/linux when no IDE CLI is found.

### Key Decisions

- `V-1` On Windows use `execFile(..., { shell: true, windowsHide: true })` so `.cmd` launchers run; quote each argument ourselves because `shell:true` disables Node's automatic escaping.
- `V-2` Wrap the spawn in try/catch and resolve to the next candidate so neither a synchronous throw nor a callback error escapes as an unhandled rejection.
- `V-3` Fall back to Electron `shell.openPath(file)` (no line targeting) when no IDE CLI launches — works on all three platforms; this is the macOS/linux robustness fix.

### Constraints

- Keep line targeting via the IDE CLIs when they are available (`code -g file:line`, `xed -l line file`).
- Do not introduce shell-injection: only enable `shell:true` on Windows and quote args.

### Open Questions

- None for this slice.

### Source Refs

- User report on `2026-06-16` with the stack trace: `spawn EINVAL` at `dist-electron/main.js` `tryOpen` / `execFile`.
- Node CVE-2024-27980 (BatBadBut): `.cmd`/`.bat` spawn requires `shell:true` on Windows.
- Desktop code: `apps/desktop-flowpilot/electron/main.ts` (`tryOpen`, `quoteWinArg`, the `ide:open` handler).

## 1. Issue Summary

The desktop "open in IDE" affordance invokes an IPC handler (`ide:open`) that tries a list of IDE CLI launchers via `execFile`. On Windows those launchers are `.cmd` shims; modern Node refuses to spawn them without a shell and throws `spawn EINVAL`, which surfaced as an unhandled promise rejection and left the file unopened. On macOS, when the `code`/`cursor` CLIs are not installed on PATH, the handler exhausted its candidates and did nothing.

## 2. Parent Links

- impacted coding plan: [04-01 Phase 1 Desktop Mock MVP](../../10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md) (Part B — real IDE open)
- impacted tech design: [03 Solution And System Design](../../10-Refactor/New-System/03-Solution-And-System-Design.md)
- impacted system spec: `none` (Electron shell behavior, not a workflow spec rule)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` Electron shell on Windows 11 (Node ≥ 18.20 / 20.12); also affects macOS when no IDE CLI is on PATH.
- reproduction steps:
  1. Run a chat turn that produces or changes a file.
  2. Click "open in IDE" on the file chip in the timeline.
  3. Observe the file does not open; the main-process console logs `UnhandledPromiseRejectionWarning: Error: spawn EINVAL`.
- frequency: always on Windows.

## 4. Expected vs Actual

- expected: the file opens in the installed IDE at the given line, or in the OS default app if no IDE CLI is found; no unhandled rejection.
- actual: `execFile` of the `.cmd` launcher threw `EINVAL`; the rejection went unhandled and nothing opened. On macOS without a CLI, nothing opened and there was no fallback.

## 5. Impact

- users affected: all Windows desktop users (and macOS users without an IDE CLI on PATH).
- workflows affected: opening generated/changed files from the run timeline.
- severity: medium — a convenience action is fully broken on Windows.

## 6. Root Cause

- hypothesis: the `execFile` comment ("not a shell — avoids quoting issues") predates the Node security change that made shell-less `.cmd` spawning illegal.
- confirmed cause: Node 18.20/20.12 (CVE-2024-27980) makes `child_process` throw `EINVAL` when spawning a `.cmd`/`.bat` without `shell:true`. The Windows IDE launchers (`code.cmd`, etc.) hit this on every candidate; the rejection was never caught.
- evidence:
  - the reported stack trace originates at `execFile` inside `tryOpen` with `spawn EINVAL`.
  - on Windows `code`/`cursor`/`studio` resolve to `.cmd` shims that require a shell.

## 7. Fix Strategy

- `F-1` In `tryOpen`, set `{ shell: process.platform === "win32", windowsHide: true }` so Windows `.cmd` launchers spawn (and bare `code` resolves via PATHEXT through the shell).
- `F-2` Add `quoteWinArg` and quote each argument on Windows, since `shell:true` disables Node's automatic argument escaping.
- `F-3` Wrap the spawn in try/catch; on a synchronous throw or callback error, fall through to the next candidate instead of rejecting.
- `F-4` In the `ide:open` handler, fall back to `shell.openPath(file)` when no IDE CLI launches — cross-platform guarantee the file opens (covers macOS/linux without a CLI on PATH).

## 8. Validation

- `V-1` `npm run typecheck --prefix apps/desktop-flowpilot` passes.
- `V-2` Manual (Windows): clicking "open in IDE" opens the file in VS Code at the line, with no `spawn EINVAL` in the main-process console.
- `V-3` (deferred) Manual macOS/linux smoke once a build is available on those platforms; the `shell.openPath` fallback is platform-native and requires no Windows-specific code.

## 9. Regression Guard

- tests: none added — `tryOpen` runs in the Electron main process and depends on platform spawn behavior + installed IDEs, which the renderer unit/typecheck harness cannot exercise. Verified via typecheck + manual Windows repro.
- alerts: none.
- audit checks: confirm `shell` is enabled only on win32 and every spawned argument is quoted on that path (no shell injection on POSIX, where `shell:false`).

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`.
- notes left unchanged on purpose: the IDE candidate list (`code`/`cursor`/`studio`/`xed`) is unchanged; only the spawn mechanics and fallback were fixed.
