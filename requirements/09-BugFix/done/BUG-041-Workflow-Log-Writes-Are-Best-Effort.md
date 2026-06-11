# BUG-041: Workflow Log Writes Are Best Effort

## Metadata

- Document ID: `BUG-041`
- Title: `Workflow Log Writes Are Best Effort`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/done/CP-16-Runner-Log.md`, `requirements/06-System-Tech-Design/SD-05-Workflow-Engine.md`
- Child Documents: `none`
- Related Documents: `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`, `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`, `change-audit/CA-052-workflow-log-write-failures-are-best-effort.md`
- Replaces: `none`
- Tags: `workflow-engine, logs, runtime, regression, reliability`

## AI Quick View

### Summary

- Background workflow execution failed when a Supabase log insert threw `TypeError: fetch failed`.
- The runtime treated workflow log writes as fatal, even though the log system is meant to be best-effort.
- The fix makes `insertLog()` swallow request and insert failures and continue execution.
- A regression test now verifies that a log write failure does not abort the workflow.

### Current Ask

- Record the runtime fix that keeps workflow execution alive when `workflow_run_logs` writes fail.

### Key Decisions

- `V-1` Workflow log writes must not abort background execution.
- `V-2` Network/request failures from the Supabase insert path should be logged and skipped, not rethrown.

### Constraints

- Preserve the existing best-effort logging contract from CP-16.
- Keep the fix local to log writes; do not change provider/session recovery behavior.

### Open Questions

- None.

### Source Refs

- `Error: Unable to write workflow run log: TypeError: fetch failed`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts:1721`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`

## 1. Issue Summary

The background workflow runtime aborted when a workflow log insert failed with a fetch-level request error. The failure surfaced as `Unable to write workflow run log: TypeError: fetch failed`, which turned a logging problem into a workflow execution failure.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-16-Runner-Log.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-05-Workflow-Engine.md`
- impacted system spec: `requirements/05-System-Specs/SS-10-Full-Flow.md`

## 3. Environment and Reproduction

- environment: admin-web workflow runtime executing a background workflow step
- reproduction steps:
  1. Force the Supabase `workflow_run_logs` insert to fail with a request-level error.
  2. Execute a workflow step that emits a log entry.
  3. Observe the runtime abort while trying to record the log.
- frequency: intermittent in environments where the Supabase request path fails

## 4. Expected vs Actual

- expected: logging failures are skipped and the workflow continues
- actual: the thrown log-write error aborted background workflow execution

## 5. Impact

- users affected: anyone running background workflows
- workflows affected: any workflow step that writes logs while the Supabase request layer is unavailable
- severity: high because observability failures should not stop execution

## 6. Root Cause

- hypothesis: `insertLog()` treated all Supabase write failures as fatal
- confirmed cause: the helper rethrew non-FK log insert failures, including request-level errors such as `TypeError: fetch failed`
- evidence:
  - stack trace points to `insertLog()` in `workflow-start-runtime.ts`
  - the failing path reports `Unable to write workflow run log: TypeError: fetch failed`

## 7. Fix Strategy

- `F-1` Wrap the `workflow_run_logs` insert in `insertLog()` with a `try/catch`.
- `F-2` Treat both Supabase error objects and request-level exceptions as non-fatal log write failures.
- `F-3` Add a regression test that simulates `fetch failed` and confirms workflow execution still succeeds.

## 8. Validation

- `V-1` `npx vitest run src/features/workflow-engine/workflow-start-runtime.test.ts` passed in `apps/admin-web`.
- `V-2` `npm run build` passed in `apps/admin-web`.
- `V-3` The new regression test verifies log-write failure does not abort `sendMessageWithRetry()`.

## 9. Regression Guard

- tests:
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
- alerts:
  - none added
- audit checks:
  - keep workflow log writes best-effort
  - do not reintroduce fatal runtime behavior for logging-only failures

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - CP-16 already defines log writes as best-effort, so no spec delta was required
