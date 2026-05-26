# CA-022: Session-Aware Workflow Runtime Core

## Scope

This audit covers the core session-aware runtime work that changed how workflow steps reuse live provider sessions instead of starting a fresh process every time.

## Completed

- Added database persistence for `workflow_run_sessions`.
- Implemented the local runner session registry and the `/sessions/start`, `/sessions/message`, and `/sessions/close` endpoints.
- Updated the runtime orchestrator so step execution can reuse a shared provider session when appropriate.
- Added isolated subagent sessions for steps that require dedicated execution contexts.

## Verification

- Verified the new session lifecycle unit tests in the Go runner.
- Confirmed the frontend runtime can resolve the correct session policy for shared and isolated steps.

