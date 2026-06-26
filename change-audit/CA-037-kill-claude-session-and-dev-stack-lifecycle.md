# CA-037: Kill Claude Process and Dev Stack Lifecycle Controls

Implement reliable session/process killing for AI provider executions (such as virtual Claude sessions) and add dev stack lifecycle control capabilities (Shutdown / Restart) through a supervisor process.

## Scope

- **Runner Session Management**: `apps/local-runner/internal/runner/sessions.go` and `sessions_test.go`.
- **Runner System Endpoints**: `apps/local-runner/internal/cli/root.go`.
- **Web App Sidebar and Gateways**: `apps/admin-web/src/components/layout/app-shell.tsx`, `apps/admin-web/src/domain/gateway/local-runner-gateway.ts`, `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`, and test mocks in `local-first-workflow-gateway.test.ts`.
- **Workflow Run Detail Page**: `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`.
- **Supervisor & Dev Tooling**: `scripts/supervisor.js` [NEW] and `Justfile`.

## Completed

- **Session Mutual Exclusion, Interruptibility & Concurrent Wait Fix**:
  - Added `SendMu` lock to Go runner `LiveSession` to serialize prompt/message sends while freeing the `Mu` lock during slow command runtimes. This prevents deadlocking the session object during long runs and allows `CloseSession(...)` to execute immediately.
  - Tracked transient processes (`Cmd` and `Pid`) in `LiveSession` during Claude executions to enable explicit interrupt requests.
  - Prevented racing `cmd.Wait()` calls concurrently across different goroutines by checking `TransportType != "claude_stream_json"` before calling `Wait()` in `CloseSession`, `CleanupSessions`, and `sweepIdleSessions`. Since Claude sessions run as short-lived commands inside `SendMessage()`, `SendMessage()` is already waiting on them and cleans them up.
  - Decreased `CloseSession` wait timeout to `200ms` to quickly terminate lingering provider processes.
- **System API Endpoints**:
  - Registered `POST /system/shutdown` and `POST /system/restart` HTTP endpoints on the local runner.
  - Endpoints clean up active provider sessions and write commands synchronously to `.flowpilot/supervisor.cmd`, flushing the `202 Accepted` response back to the client immediately.
- **Lifecycle Supervisor**:
  - Created `scripts/supervisor.js` process supervisor. It launches the Next.js web application and Go local-runner, writes process PIDs to `.flowpilot/supervisor.json`, and handles terminal shutdown/restart signals.
  - **Graceful Termination & Signal Trapping**: When a `SIGINT` (Ctrl+C) or `SIGTERM` is received, the supervisor gives child processes a 3-second grace period to shut down cleanly on their own before force-killing them.
  - **Detached Process Groups on Unix**: Spawned processes with `detached: true` on Unix and targeted their process groups (`-pid`) during kills to prevent orphaned child processes.
  - **Active Port PID Discovery & Ownership Checks**: Added port PID lookup using `netstat` (Windows) and `lsof`/`ss` (Unix) to dynamically resolve PIDs of pre-existing stack processes if the supervisor is restarted. Verified the PIDs' command line commands (`wmic` on Windows, `ps`/`/proc` on Unix) to prevent signaling/killing unrelated processes on PID reuse.
- **Web Interface Updates**:
  - Added `shutdownStack()` and `restartStack()` endpoints to `LocalRunnerGateway` interface, `HttpLocalRunnerGateway` implementation, and `local-first-workflow-gateway.test.ts` test double.
  - Rendered `Shutdown` and `Restart` buttons in the sidebar footer of `app-shell.tsx` when `runnerOnline` is true, handling pending states and health polling loops.
  - Enabled "Kill Process" button on workflow run detail page `$runId.tsx` for sessions based on `processKey` (instead of requiring `processPid` to be non-null), allowing virtual Claude sessions to be killed.

## Verification

- **Automated Tests**:
  - Added Go unit tests `TestCloseSessionRacingWithClaudeStart` and `TestSweepIdleSessionsSkipsInFlightSession` to `sessions_test.go` to verify execution safeties.
  - Verified that all Go runner tests passed (`go test -v -count=1 ./internal/runner`).
  - Verified that all admin-web frontend Vitest tests passed (`npm run test`).
  - Verified that Go runner builds successfully (`go build ./...`).
  - Verified that the Next.js admin-web production build compiles successfully (`npm run build`).

## Residual Notes

- Running the stack locally requires executing `just dev`, which now uses the supervisor to orchestrate Next.js and Go processes.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: CA-037
change_type: feature
summary: Kill Claude Process and Dev Stack Lifecycle Controls
# --->8---
