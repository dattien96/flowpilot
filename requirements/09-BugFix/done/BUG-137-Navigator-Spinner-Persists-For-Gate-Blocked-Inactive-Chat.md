# BUG-137: Navigator Spinner Persists For Gate-Blocked Inactive Chat

## Metadata

- Document ID: `BUG-137`
- Title: `Navigator Spinner Persists For Gate-Blocked Inactive Chat`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md)
- Child Documents: none
- Related Documents: [BUG-138](./BUG-138-Gate-Block-Modal-Re-Pops-Every-Chat-Open.md), [CA-126](../../../change-audit/CA-126-fix-gate-block-ui-and-r-bug-action.md)
- Replaces: none
- Tags: `context-regression-engine, ui, navigator, flow-gate, regression`

## AI Quick View

### Summary

- After the flow gate hard-blocks a chat, Navigator correctly shows no spinner while the user stays on that chat (live `status = "completed"`). But when the user switches to a different chat, the blocked chat's navigator entry switches to `item.status` from the polled `runHistory`, which the runner still reports as `"running"` (the run is not formally closed — it's waiting for the user's next prompt).
- Symptom: the spinning loading icon reappears on the blocked chat's navigator entry every time another chat is active.

### Current Ask

- Suppress the spinner for gate-blocked inactive chats without requiring a runner-side status update.

### Key Decisions

- `D-1` Track `_gateBlockedRunIds: Record<string, boolean>` in the Zustand store. When a live `flow_gate_violation` with `status === "block"` fires, add the run ID. When `turn_started` fires for that run (user re-prompted), remove it.
- `D-2` Navigator uses `gateBlockedRunIds[item.runId] ? "completed" : item.status` for the `effectiveStatus` of non-active history items.

### Constraints

- Must not affect genuinely running chats in other projects.
- `_gateBlockedRunIds` must NOT be cleared on chat switch (that's the whole point).

### Open Questions

- Q-1: Should the runner update the run status to `"idle"` after a gate block? That would fix the API-level stale status and make the frontend workaround unnecessary. Deferred (runner status lifecycle change).

### Source Refs

- Code: `apps/desktop-flowpilot/src/state/store.ts` (`applyEvent`, `_gateBlockedRunIds`), `apps/desktop-flowpilot/src/components/Navigator.tsx` (`effectiveStatus`).

## 1. Issue Summary

The Navigator left-panel spinner icon shows for a gate-blocked chat whenever it is not the active chat. The runner keeps the run in `"running"` state after a gate block (no explicit status update is made — the run awaits the user's next prompt). The `loadRunHistory` poll returns `status: "running"` for that run, and Navigator's `effectiveStatus` for non-active items uses `item.status` directly.

## 2. Parent Links

- impacted coding plan: CP-35
- impacted tech design: SD-20 §3 (UX behavior table)
- impacted system spec: SS-14

## 3. Environment and Reproduction

- environment: Desktop app, any project with test failures that trigger r-tests/r-reg gate block
- reproduction steps: (1) Trigger a gate block on a chat. (2) Switch to a different chat. (3) Observe the blocked chat's navigator entry — spinner shows.
- frequency: 100% when gate hard-blocks

## 4. Expected vs Actual

- expected: Blocked chat shows no spinner (it's not running — it's waiting for user input)
- actual: Spinner shows whenever the blocked chat is not the active chat

## 5. Impact

- users affected: Any user who hits an r-tests/r-reg gate block and switches chats
- workflows affected: Context & Regression Engine (CP-35)
- severity: Medium — confusing UX, not data loss

## 6. Root Cause

- hypothesis: Navigator uses `item.status` from polled `runHistory` for non-active items; runner doesn't update run status after gate block
- confirmed cause: Runner keeps the run in `"running"` state after a gate block (no `finalizer.Finalize` is called, so no status transition occurs). Navigator's `effectiveStatus = isActive ? status : item.status` falls through to the stale polled value.
- evidence: Code inspection of `gate_hook.go` (no status update on `block`) and `Navigator.tsx` (`effectiveStatus` logic).

## 7. Fix Strategy

- `F-1` Add `_gateBlockedRunIds: Record<string, boolean>` to `AppState` in `store.ts`, initialized to `{}`.
- `F-2` In `applyEvent`: when `flow_gate_violation` with `status === "block"` and `!_historyReplaying`, add `e.workflowRunId` to `_gateBlockedRunIds`. When `turn_started` fires, remove `e.workflowRunId` from `_gateBlockedRunIds`.
- `F-3` In `Navigator.tsx`: subscribe to `_gateBlockedRunIds`; compute `effectiveStatus = isActive ? status : (gateBlockedRunIds[item.runId] ? "completed" : item.status)`.

## 8. Validation

- `V-1` Trigger a gate block → switch to another chat → blocked chat shows no spinner. ✓ (manual)
- `V-2` Send a new prompt on the blocked chat → spinner reappears correctly (back to running). Requires test.
- `V-3` Non-blocked running chats still show their spinner correctly. ✓ (manual)

## 9. Regression Guard

- tests: No automated test added; covered by manual E2E-8 reproduction.
- alerts: none
- audit checks: `_gateBlockedRunIds` must be cleared per-run by `turn_started`, not globally.

## 10. Follow-Up Document Updates

- SD-20 §3 UX table: no change needed (the fix is a frontend detail, not a semantic change).
- Q-1 (runner-side status update): deferred — a runner fix would make `_gateBlockedRunIds` redundant and could be landed as a separate task.
