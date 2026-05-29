# BUG-015: Can Not Kill Process If Real Id Was Not Created

## Status:
- Fixed in code
- Verified by manual testing

## Problem:
- We can only kill a process when the runner has a real PID or an attached live session key.
- Right after a task is triggered, the UI can show a temporary session id:
  - `claude_stream_session_prompt_xxx` for Claude
  - `codex_mcp_session_prompt_yyy` for Codex
  - similar temporary ids for Gemini
- If kill is pressed in that phase, the system must still stop the active run correctly and must not auto-resume it.
- After the first response arrives and the provider returns the real session id, kill must still work.

## Verified test cases:
- `Ctrl-C` stops the system and kills active AI processes.
- `Stop` by the workflow-run list UI button kills the active session.
- `Kill Process` by the detail page works for active sessions.
- `DEL` workflow run from the list kills active sessions.
- `Stop server` from the menu kills active AI processes.
- `Restart server` from the menu kills active AI processes and starts the stack again.
- Kill during the initial temp-session phase now stops the session instead of reconnecting it.
- Kill after the first real provider session id is created still works.

## Observed during testing:
- I hit a separate workflow runtime error while testing:
  - `Unable to write workflow run log: insert or update on table "workflow_run_logs" violates foreign key constraint "workflow_run_logs_workflow_run_step_id_fkey"`
- That means workflow log writes must not abort the workflow when the step row is already missing.
- The runtime now treats that log insert as best-effort for the missing-step case.

# Conclusion:
- The kill flow is now stable across both phases:
  - temporary session id phase
  - real provider session id phase
- The remaining workflow log issue is handled as non-fatal for missing step rows.
