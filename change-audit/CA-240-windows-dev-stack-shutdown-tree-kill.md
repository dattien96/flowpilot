# CA-240: Windows Dev-Stack Shutdown Now Tree-Kills The Compiled Runner Binary

## Summary

BUG-240: on Windows, stopping the dev stack (Ctrl-C, or the desktop app's "Turn off system" control) orphaned the compiled Go runner binary (`flowpilot.exe`, built by `go run` into a temp dir) because the supervisor only ever tracked and signaled the `go.exe`/`cmd.exe` wrapper PID, and the tree-kill fallback (`taskkill /F /T`) was gated behind a liveness poll on that same wrapper PID — which exits quickly on Windows, so the poll read "dead" and skipped the tree-kill before it could catch the still-running compiled binary. macOS/Linux were unaffected (POSIX process-group signaling already reaches the whole tree). Fixed by making the Windows branch of `signalManagedProcess` call the existing tree-kill immediately instead of a single-PID signal.

## What Changed

### Dev tooling (`scripts/`)

- `scripts/supervisor.js` `signalManagedProcess`: added a `process.platform === 'win32'` branch that calls `killProcessTree(child)` (the pre-existing `taskkill /F /T /PID <pid>` implementation) immediately, instead of the previous `process.kill(child.pid, signal)` which only ever reached the tracked wrapper PID and never its compiled-binary descendant. POSIX behavior (process-group `SIGINT` via `detached`) is unchanged. All three call sites that funnel through this function — `stopManagedProcess`, `cleanupAndExit`, and the restart-flow cleanup — inherit the fix without their own changes.

### Docs

- `requirements/09-BugFix/done/BUG-240-Windows-Dev-Stack-Shutdown-Orphans-Compiled-Runner-Binary.md` — root cause, fix, and manual process-tree validation (pre-fix repro + post-fix confirmation).

## Verification

- `node --check scripts/supervisor.js` — syntax clean.
- Manual process-tree reproduction (no automated test suite exists for this dev-tooling script): spawned `go run ./cmd/flowpilot runner serve --port 8799` exactly as the supervisor does, confirmed the compiled `flowpilot.exe` is a real Windows child process of the tracked `go.exe` wrapper PID via `Get-CimInstance Win32_Process`. Pre-fix `process.kill(wrapperPid, 'SIGINT')` left the compiled binary running after the wrapper died. Post-fix `taskkill /F /T /PID <wrapperPid>` terminated the wrapper, the compiled binary, and an intermediate shell layer together — nothing left behind.
- Not executed: a full `npm run dev` stack start/stop cycle in the actual desktop app (validated the exact spawn/kill mechanics directly instead, since that's the part that changed).

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: BUG-240
change_type: bugfix
summary: Tree-kill the Windows dev-stack processes on shutdown instead of signaling only the go run wrapper PID, so the compiled runner binary no longer survives as an orphan
# --->8---
