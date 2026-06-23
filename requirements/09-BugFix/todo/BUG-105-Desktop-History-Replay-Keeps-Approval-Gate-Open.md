## Metadata

- Document ID: `BUG-105`
- Title: `Desktop History Replay Keeps Approval Gate Open`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-21`
- Last Updated: `2026-06-21`
- Parent Documents: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)
- Child Documents: none
- Related Documents: [BUG-100: YOLO Off Spawn Agent Child Approval Hangs Without Surfacing The Gate](../done/BUG-100-Yolo-Off-Spawn-Agent-Child-Approval-Hangs-Without-Surfacing-The-Gate.md), [BUG-104: Desktop Codex Child Spawn Failed On Unsupported Priority Tier](./BUG-104-Desktop-Codex-Child-Spawn-Failed-On-Unsupported-Priority-Tier.md)
- Replaces: none
- Tags: desktop, approval, history, replay, regression

## AI Quick View

### Summary

- A history replay that ends on `permission_required` can currently be finalized as `resolved`, which hides the action buttons.
- This is visible after switching views or reopening a run whose replay is truncated at the approval gate.
- The replay should keep the approval card actionable when the gate is still the last visible event.

### Current Ask

- Fix desktop history replay so a still-open approval gate stays open instead of being stamped `resolved`.

### Key Decisions

- `V-1` A replay that ends on an unresolved approval gate must keep `pendingApproval` and preserve the approval buttons.

### Constraints

- Do not convert YOLO-off approval into auto-approval.
- Do not hide the approval card when the replay is still sitting on the gate.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts`
- `apps/desktop-flowpilot/src/state/store.test.ts`
- SD-16 Test 4 notes

## 1. Issue Summary

When the desktop replays or restores a run that stops at a `permission_required` event, the UI can stamp the approval card as `resolved` and remove the action buttons. The user then sees the approval card as read-only even though the run is still sitting at the gate.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot, Codex provider, YOLO off
- reproduction steps:
  1. Open a run that reaches a child or main approval gate.
  2. Switch between main and child views or reopen the run from history.
  3. Stop the replay on the `permission_required` event.
  4. Observe that the card becomes `resolved` and the buttons disappear.
- frequency: reproducible when replay ends on the gate

## 4. Expected vs Actual

- expected: the approval card stays open with `Approve` and `Deny` actions while the gate is still pending
- actual: the card is stamped `resolved`, which hides the approval buttons

## 5. Impact

- users affected: desktop users working through approval-gated runs
- workflows affected: child-agent approval, history reopen, main/child view switching
- severity: high, because the user loses the ability to continue the gated run from the UI

## 6. Root Cause

- hypothesis: the replay finalizer treats any non-waiting replay as settled even when the last visible event is still `permission_required`
- confirmed cause: the desktop clears pending approval state after replay ends instead of checking whether the replay stopped on the gate itself
- evidence: the approval card is rendered from a `decision` field, and the replay path currently stamps unresolved approval cards as `resolved` when it believes replay is finished

## 7. Fix Strategy

- `F-1` Detect whether the last meaningful replay item is still an unresolved approval gate.
- `F-2` Preserve `pendingApproval` and keep the status at `waiting_approval` when replay ends on that gate.

## 8. Validation

- `V-1` `npm --prefix apps/desktop-flowpilot run typecheck` passed.
- `V-2` `apps/desktop-flowpilot/node_modules/.bin/tsc -p tsconfig.phase1-tests.json` passed.
- `V-3` Added a store regression test that reopens a run whose replay ends on `permission_required` and asserts the approval gate stays open.
- `V-4` `npm --prefix apps/desktop-flowpilot run test:phase1` still hits unrelated pre-existing harness failures (`@supabase/supabase-js` resolution and an unrelated `timelineSkillSummary` path issue) before this slice can finish end to end.

## 9. Regression Guard

- tests: `openHistoryRun keeps an approval gate open when the replay ends on permission_required`
- alerts: none
- audit checks: replay finalization must never hide an unresolved approval gate

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: approval semantics stay manual when the gate is still live
