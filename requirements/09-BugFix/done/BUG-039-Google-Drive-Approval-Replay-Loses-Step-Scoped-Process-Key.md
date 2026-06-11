# BUG-039: Google Drive Approval Replay Loses Step-Scoped Process Key

## Metadata

- Document ID: `BUG-039`
- Title: `Google Drive Approval Replay Loses Step-Scoped Process Key`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`
- Child Documents: `none`
- Related Documents: `change-audit/CA-043-cp29-google-drive-mcp-manual-approval-ui.md`, `change-audit/CA-047-fix-google-drive-approval-replay-process-key.md`
- Replaces: `none`
- Tags: `workflow-engine, google-drive, approval, session-recovery, cp-29, regression`

## AI Quick View

### Summary

- CP-29 binds Google Drive proxy approvals to the active workflow run, workflow step run, and FlowPilot process key.
- Approving a waiting Google Drive request should replay the exact same MCP tool call and continue the paused step.
- When the provider session had to recover after `session_dead` or bootstrap replay, the replay path started a new session without restoring the deterministic step-scoped process key.
- That broke the lookup for the already-approved proxy request and caused the run to stall in approval flow instead of continuing cleanly.

### Current Ask

- Record the fix so Google Drive approval replay keeps the step-scoped FlowPilot process key across session recovery and can consume the approved proxy request instead of drifting into a new approval context.

### Key Decisions

- `V-1` Session recovery for Google Drive step-scoped runs must preserve the deterministic `FLOWPILOT_PROCESS_KEY`.
- `V-2` Approval replay must continue to use the exact step-scoped approval identity after `session_dead` and bootstrap replay recovery.

### Constraints

- Keep CP-29 approval scoping rules unchanged; do not relax exact-match approval binding.
- Keep the fix scoped to session recovery and replay; do not redefine YOLO or generic approval behavior.
- Do not claim end-to-end runtime verification that could not be executed in the current shell.

### Open Questions

- Should the run detail UI surface a clearer operator message when a resumed session creates a fresh approval context instead of consuming an existing approved request?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
- user report on `2026-06-11`: approved Google Drive request did not resume and the run stayed in waiting state

## 1. Issue Summary

Google Drive proxy approvals in CP-29 are scoped to the exact workflow run, workflow step run, and FlowPilot process key. A paused run could accept the operator's `approved` decision, but the replay path still failed to continue when the underlying provider session had to recover first. Instead of reusing the same scoped approval identity, the recovered send path could drift into a fresh process context, so the already-approved request was no longer matched during replay.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- impacted system spec: `requirements/05-System-Specs/SS-04-Workflow.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: FlowPilot admin-web runtime using CP-29 Google Drive proxy approvals with a paused workflow step that later resumes through follow-up replay
- reproduction steps:
  1. Run a Google Drive MCP workflow step with YOLO disabled so the proxy creates a pending approval and pauses the step.
  2. Leave the step paused long enough that the original provider session may require recovery, or trigger a recovery path such as `session_dead`.
  3. Approve the waiting Google Drive request from the run detail UI.
  4. Observe the replay path after approval.
- frequency: conditional but repeatable whenever approval replay enters the session recovery path

## 4. Expected vs Actual

- expected: approving the exact pending Google Drive request should replay that same tool call under the same scoped approval identity and let the paused workflow continue
- actual: the recovery path could start the resumed provider session without the deterministic step-scoped process key, so the replay no longer matched the approved request and the workflow remained stuck in approval flow

## 5. Impact

- users affected: operators approving paused Google Drive MCP workflow steps after the provider session has drifted or died
- workflows affected: CP-29 paused Google Drive approval replay, especially long-lived or recovered runs
- severity: high because the operator sees approval succeed but the paused workflow still does not continue reliably

## 6. Root Cause

- hypothesis: session recovery logic for follow-up replay preserved provider thread identity but forgot to preserve FlowPilot's approval-scoping process key
- confirmed cause: `sendMessageWithRetry` passed the deterministic step-scoped process key only on the initial Google Drive send path. The `session_dead` and bootstrap replay recovery branches called `getOrCreateSession` without `requestedProcessKey`, which let recovered sessions start without the exact same `FLOWPILOT_PROCESS_KEY`
- evidence:
  - Google Drive proxy approvals are matched by workflow run id, workflow step run id, process key, tool name, and arguments
  - `sendMessageWithRetry` defines a deterministic step-scoped process key for Google Drive sessions
  - the initial `getOrCreateSession` call received `requestedProcessKey`, but the recovery branches did not
  - approval replay after an `approved` decision depends on recovering the same scoped approval identity

## 7. Fix Strategy

- `F-1` Pass the deterministic step-scoped `requestedProcessKey` through the `session_dead` recovery branch in `sendMessageWithRetry`.
- `F-2` Pass the same deterministic step-scoped `requestedProcessKey` through the bootstrap replay recovery branch so both recovery modes preserve approval scope.
- `F-3` Add a focused regression test that simulates Google Drive `session_dead` recovery and asserts the replayed session keeps the same `FLOWPILOT_PROCESS_KEY`.

## 8. Validation

- `V-1` Code inspection confirms the recovery calls in `sendMessageWithRetry` now preserve `requestedProcessKey: stepScopedProcessKey` for both same-thread recovery and bootstrap replay.
- `V-2` Added a focused runtime regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts` that asserts Google Drive `session_dead` recovery keeps the exact same `FLOWPILOT_PROCESS_KEY`.
- `V-3` Attempted `npx vitest run apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`, but this shell still fails before test execution because direct file runs are not resolving the repo's `@/...` aliases in this environment.

## 9. Regression Guard

- tests:
  - added a focused send/recovery regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
  - automated execution of that test remains blocked in the current shell by repo alias resolution
- alerts:
  - none added
- audit checks:
  - keep all Google Drive step-scoped recovery paths aligned on the same deterministic FlowPilot process key
  - when new session recovery modes are added, verify they preserve approval scope for exact-match proxy approvals

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - none required because the implementation is being brought back into alignment with existing CP-29 approval scoping rules
- notes left unchanged on purpose:
  - CP-29 remains the source of truth for exact request approval scope and replay behavior
