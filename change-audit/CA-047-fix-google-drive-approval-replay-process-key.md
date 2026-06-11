# CA-047 Fix Google Drive Approval Replay Process Key

## Scope

Fixed the CP-29 approval replay regression where a paused Google Drive MCP step could accept an `approved` decision but still fail to continue after provider session recovery because the recovered replay no longer used the same FlowPilot process key.

## Completed

- Updated `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts` so `sendMessageWithRetry` now preserves the deterministic step-scoped `requestedProcessKey` in both `session_dead` recovery and bootstrap replay recovery paths.
- Kept the fix scoped to Google Drive step-scoped recovery behavior; no CP-29 approval semantics, YOLO rules, or generic approval flow rules were changed.
- Added a focused regression test in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts` that simulates a Google Drive `session_dead` recovery and asserts the resumed session keeps the exact same `FLOWPILOT_PROCESS_KEY`.
- Added the formal bug record at `BUG-039` documenting the approval replay regression and the recovered-session scope fix.

## Verification

- Code inspection of the recovered session branches in:
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
- Added a focused runtime regression test for Google Drive session recovery scope:
  - `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`
- Attempted targeted test execution:
  - `npx vitest run apps/admin-web/src/features/workflow-engine/workflow-start-runtime.test.ts`

## Residual Notes

- The targeted Vitest invocation in this shell still fails before test discovery because direct file runs are not resolving the repo's `@/...` path aliases in this environment.
- GitNexus tools were not available in this thread, so the repo's normal symbol impact analysis flow had to be replaced with careful local inspection only.
- The existing `workflow-start-runtime` test file also contains the previously added single-step YOLO regression coverage from the earlier fix in this branch.
