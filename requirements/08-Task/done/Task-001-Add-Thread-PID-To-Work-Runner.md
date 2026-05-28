# Task-001: Add Thread PID To Work Runner

## Goal
When a workflow step starts a subprocess, capture the subprocess PID, persist it with the workflow session, and expose it in the workflow UI for tracking and manual termination.

## Requirements
The runner currently starts a subprocess for a step run.

We need to:

1. Detect the OS context: `windows` or `mac/linux`.
2. Capture the spawned subprocess PID when the step session starts.
3. Persist that PID so it can be loaded later with the workflow run detail data.
4. Show the PID in the workflow run detail UI.
5. Add a manual `Kill Process` action for an individual active session.
6. Add a workspace-level `Kill All Processes` action on the workflow run history page.
7. Count all active sessions across workflow runs, including main sessions and replay sessions.

## Data Behavior
- PID must come from the real spawned subprocess, not a generated value.
- PID must be associated with the correct session or step run.
- If a session is no longer active, the PID must be hidden in the UI even if the database still stores an old value.
- If a PID is unavailable for a specific provider or OS path, store `null` and do not break the flow.
- The session row should continue to track `process_key`, `provider_session_id`, `status`, and timestamps.
- Manual kill and bulk kill should update the persisted session row to `completed` and clear `process_key` after the process is closed.

## UI Surfaces
### Workflow Run Detail
Display the PID inside the session box for the related step.

Expected placement:
- Current session header already shows:
  - session scope
  - provider/model
  - session status
  - provider session/thread id
- Show the PID only when the session is `active`.
- Add a small `Kill Process` button beside the PID for manual termination.

Example target display:
- `Session: SESSION MAIN - codex (gpt-5.4-mini) - active`
- `PID 12345 Kill Process`
- `Session ID 019e...`

### Workflow Run History
Add a header indicator that shows the number of active processes in the workspace.

Expected placement:
- Count all active workflow sessions with a live `process_key`.
- Include both main sessions and replay sessions.
- Add a `Kill All Processes` button beside the counter.
- The button should terminate every active session and update the count after completion.

## Scope
- Runner process start logic
- Local runner close-session logic
- Session or step-run persistence model
- Workflow run detail data loading
- Workflow run detail session box UI
- Workflow run history header counter and bulk kill action

## Acceptance Criteria
- Starting a workflow step stores the subprocess PID.
- Reloading the workflow run detail page still shows the same PID while the session remains `active`.
- The PID is visible directly inside the session box for the related step only when the session is `active`.
- The `Kill Process` button is displayed beside the PID only when the session is `active`.
- The workflow run history page shows an accurate count of active processes across all workflow runs.
- The `Kill All Processes` button terminates every active session in the workspace.
- The solution works across both `windows` and `mac/linux` execution paths.
