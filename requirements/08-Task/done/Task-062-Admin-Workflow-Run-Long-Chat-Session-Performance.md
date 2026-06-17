# Task-062: Admin Workflow Run Long Chat Session Performance

## Metadata

- Document ID: `Task-062`
- Title: `Admin Workflow Run Long Chat Session Performance`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-029: Workflow Runs Detail Page UX Refinements](./Task-029-Workflow-Runs-Detail-Page-UX-Refinements.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- Child Documents: [CA-086: Admin Workflow Run Chat Session Windowing](../../../change-audit/CA-086-admin-workflow-run-chat-session-windowing.md)
- Related Documents: [08-Desktop-Chat-New-Plan](../../10-Refactor/New-System/08-Desktop-Chat-New-Plan.md)
- Replaces: `none`
- Tags: `admin-web, workflow-runs, session-ui, chat-performance, windowing`

## AI Quick View

### Summary

- Improve long workflow run session UI performance when one chat session contains many prompt-response groups.
- Keep the newest/current prompt-response groups visible first while paging older groups into the DOM on demand.
- Add browser render deferral and memoization for heavy chat/output panels.
- Preserve the existing workflow run detail data contracts; this task is a render-side slice.

### Current Ask

- Done. The admin workflow run detail page now windows session prompt groups and records the result in CA-086.

### Key Decisions

- `T-1` Window prompt-response groups from the end of the chronological list so active/current chat work remains visible.
- `T-2` Page older prompt-response groups in fixed-size batches with a `Load earlier prompts` control.
- `T-3` Use render-side optimization first; defer backend/API paging until payload size is proven to be the main bottleneck.

### Constraints

- Keep the change scoped to `apps/admin-web` workflow run detail rendering.
- Do not change workflow run, output, log, artifact, or session persistence contracts.
- Do not alter prompt/output ordering semantics.

### Open Questions

- Should a future task add API/data-boundary paging for workflow outputs and logs if very large runs are still slow before first render?

### Source Refs

- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
- `apps/admin-web/src/features/workflow-engine/workflow-run-session-windowing.ts`
- `change-audit/CA-086-admin-workflow-run-chat-session-windowing.md`

## 1. Goal

Reduce UI cost for long workflow run chat sessions where a single session can contain many prompt-response groups, without changing backend data contracts or historical ordering.

## 2. Parent Links

- coding plan: closest prior task lineage is [Task-029: Workflow Runs Detail Page UX Refinements](./Task-029-Workflow-Runs-Detail-Page-UX-Refinements.md)
- tech design: [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: workflow run session UI, prompt-response session history, long-running workflow visibility

## 3. Trigger

The user identified that one chat/session section can become long and asked how to handle prompt-response display performance, including showing, preloading, caching, and paging.

## 4. Exact Change

- `T-1` Add session prompt-group windowing helpers with a fixed page size of six groups.
- `T-2` Render only the most recent prompt-response groups for each session initially.
- `T-3` Add a `Load earlier prompts` control to page older prompt-response groups into the DOM.
- `T-4` Add `content-visibility: auto` with intrinsic sizing to prompt-response group containers.
- `T-5` Memoize heavy chat bubble and output-tab components used by the workflow run detail page.
- `T-6` Add unit tests for recent-first windowing and incremental page expansion.

## 5. Touched Areas

- files:
  - `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
  - `apps/admin-web/src/features/workflow-engine/workflow-run-session-windowing.ts`
  - `apps/admin-web/src/features/workflow-engine/workflow-run-session-windowing.test.ts`
  - `change-audit/CA-086-admin-workflow-run-chat-session-windowing.md`
- modules: `admin-web`, `workflow-engine`
- routes: `/workflow-runs/$runId`
- tables: `none`

## 6. Acceptance Check

- Long sessions initially render the latest prompt-response groups instead of every historical group.
- Older prompt-response groups can be revealed by clicking `Load earlier prompts`.
- Active/live prompt thinking logic still uses the original prompt-group index, not the paged index.
- Offscreen prompt-response groups are eligible for browser layout/paint deferral.
- Focused unit tests for session windowing pass.
- Existing workflow run timeline grouping tests continue to pass.

## 7. Out of Scope

- Backend/API pagination for workflow outputs, logs, artifact runs, or session rows.
- Persistent client cache or SWR/react-query migration for run detail data.
- Changes to provider sessions, runner events, database schemas, or artifact storage.
- Redesigning the workflow run detail page beyond long-session render performance.

## 8. Completion Notes

- result: Implemented render-side prompt-response windowing, offscreen render deferral, memoization, focused unit tests, and CA-086 audit record.
- follow-ups: Add data-boundary paging/caching if large runs are still slow before React rendering begins.
- upstream docs updated: `Task-062`, `CA-086`; no system spec or tech design behavior changes required for this render-side slice.
