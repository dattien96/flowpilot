# CA-086: Admin Workflow Run Chat Session Windowing

## Scope

Admin workflow run detail UI in `apps/admin-web`, focused on long prompt-response session rendering performance.

## Completed

- Added prompt-group windowing for each workflow run session so long sessions initially render the most recent prompt-response groups instead of every historical group.
- Added a `Load earlier prompts` control that pages older prompt-response groups into the DOM in fixed-size batches.
- Kept current/live prompt groups visible by windowing from the end of the chronological group list.
- Added `content-visibility: auto` and intrinsic sizing to prompt-response group containers so the browser can skip layout and paint work for offscreen groups.
- Memoized heavy chat/output display components to reduce repeat rendering when parent session state refreshes.
- Extracted the paging math into `workflow-run-session-windowing.ts` with unit coverage.

## Verification

- `npm test -- workflow-run-session-windowing.test.ts workflow-run-detail-timeline.test.ts workflow-runs.test.tsx` passed in `apps/admin-web`.
- Filtered TypeScript check reported no errors for `workflow-runs/$runId.tsx` or `workflow-run-session-windowing`.
- Full `npx tsc --noEmit` is still blocked by existing unrelated TypeScript errors across the admin app.
- Focused ESLint is blocked because `eslint-config-next` is not installed for `apps/admin-web`.
- GitNexus impact checks for `SessionGroupSection` and `WorkflowRunDetailPage` reported LOW risk with zero affected processes/modules after re-indexing.

## Residual Notes

- This is a render-side performance slice. It does not yet add backend/API pagination or persisted prompt-response caches.
- The next performance layer, if needed, should page workflow outputs/logs at the data boundary so very large runs are not fully loaded before rendering.

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-086
change_type: feature
summary: Admin Workflow Run Chat Session Windowing
# --->8---
