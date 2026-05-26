# CA-017: Session-Aware Workflow Runtime

## Scope

Introduce a session-aware workflow execution policy that optimizes runtime execution. Instead of spawning a fresh command process for every step and follow-up prompt, FlowPilot now maintains and reuses live provider session processes (for non-subagent steps) while isolating execution contexts for steps configured with dedicated subagents.

## Completed

- **Database Persistence & Schemas:**
  - Registered `workflow_run_sessions` table via migration [20260526080000_add_workflow_run_sessions.sql](file:///c:/working/flowpilot/supabase/migrations/20260526080000_add_workflow_run_sessions.sql) to track active and historic runner sessions.
  - Linked database sessions back to `workflow_runs` detail.
  - Exposed session list queries inside `supabase-gateway-bundle.ts` and mapping definitions.

- **Local Runner API & Session Registry:**
  - Implemented a thread-safe in-memory session registry inside the Go runner (`sessions.go`), allowing initialization, prompt submission, and process clean-ups for long-lived processes.
  - Provided direct adapters for **Gemini** (ACP JSON-RPC mode), **Codex** (MCP server), and **Claude** (JSON stream mode).
  - Exposed `/sessions/start`, `/sessions/message`, and `/sessions/close` endpoints in the local HTTP server.

- **Session Policy Resolution:**
  - Updated the frontend runtime orchestrator (`workflow-start-runtime.ts`) to resolve the session policy: steps containing a `subagent` run isolated step-scoped sessions, while standard steps reuse the main workflow session.
  - Re-routed step triggers and follow-up submissions through `sendMessageWithRetry`, which automatically handles session acquisition, persistence, and fallback to one-shot execution.

- **UI Timeline & Diagnostic Details:**
  - Displayed the active session metadata banner directly in the step cards inside the run detail timeline view (`$runId.tsx`), highlighting whether the session is Shared or Isolated, its model/provider, and active status.
  - Added documentation tooltips to the step creation and edit screens (`$stepType.tsx`, `create.tsx`) regarding subagent session isolation.

## Verification

- **Backend Unit Tests:**
  - Added session lifecycle unit tests in `sessions_test.go` and verified concurrent request locking. All Go tests pass.
- **Frontend Unit Tests & Integration:**
  - Ran vitest frontend suite; all 145 tests pass successfully.
- **Production Build:**
  - Confirmed the TypeScript compilation outputs zero compiler errors (`npm run build`).

## Residual Notes

- Local runner sessions are preserved in memory during runner lifetime. If the local runner is killed or restarted, active provider processes are torn down, but subsequent actions on the UI automatically trigger graceful recovery and baseline a fresh session.
