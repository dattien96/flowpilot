# BUG-079: Desktop Live Approval Stamped Resolved On Run Switch

## Metadata

- Document ID: `BUG-079`
- Title: `Desktop Live Approval Stamped Resolved On Run Switch`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md), [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)
- Child Documents: `none`
- Related Documents: [BUG-074: Desktop History Replay Leaves Resolved Approvals Pending](./BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md), [BUG-078: Desktop Sidebar History Does Not Auto-Update](./BUG-078-Desktop-Sidebar-History-Does-Not-Auto-Update.md), [CA-089: Fix Desktop History Replay Stale Approval State](../../../change-audit/CA-089-fix-desktop-history-replay-stale-approval-state.md), [CA-094: Fix Desktop Sidebar History Auto-Update](../../../change-audit/CA-094-fix-desktop-sidebar-history-auto-update.md), [CA-095: Fix Desktop Live Approval Stamped Resolved On Run Switch](../../../change-audit/CA-095-fix-desktop-live-approval-stamped-resolved-run-switch.md)
- Replaces: `none`
- Tags: `desktop, approval, run-switch, concurrent-stream, regression`

## AI Quick View

### Summary

- Navigating away from a live `waiting_approval` run (chat A) to another run (chat B) and back stamps the approval card as `decision: "resolved"` even though the user never approved.
- Root cause: the original `consumeStream("run-A")` goroutine stays alive (blocked in `for await`, waiting for the server's next event) while the user is on chat B. When the user returns to chat A, `runId` in the store becomes `"run-A"` again so the stale guard `shouldApplyRunEvent(get().runId, runId)` passes for the old stream. The new replay stream correctly sets `pendingApproval` via `permission_required`. When the old stream then receives any event (e.g. a server reconnect notification triggered by `resumeRun`), BUG-074 F-1 stale detection fires on it — `pendingApproval` is set → approval card stamped `"resolved"` → `pendingApproval` cleared. The AI then hangs because the server is still waiting for an approval the client can no longer issue.
- The BUG-074 F-1 stale detection invariant ("if `pendingApproval` is set when an event arrives it was resolved in a prior session") is violated when two concurrent streams exist for the same `runId`.
- The existing runId-based stale guard (`shouldApplyRunEvent`) cannot distinguish "old stream for run-A" from "current stream for run-A" because both match the same `runId`.
- This bug was latent before BUG-078 (sidebar auto-update): users previously waited for a run to finish before switching, so concurrent streams never occurred in practice. BUG-078 made mid-run switching the natural pattern, surfacing the issue.

### Current Ask

- Fixed. A stream-generation counter `_streamRunSeq` is incremented each time a new `consumeStream` starts for a run. Each stream instance captures its own sequence value and exits the moment the counter advances, preventing any prior stream from applying events.

### Key Decisions

- `V-1` The fix follows the same pattern as `_historyLoadSeq` (BUG-060 F-3): a monotonic counter captured at stream start, checked on every event.
- `V-2` `_streamRunSeq` is incremented only in `openHistoryRun` and `reconnect` — the two paths where a new stream starts for a potentially already-active `runId`. `sendPrompt` starts streams for a fresh `runId` so the existing `shouldApplyRunEvent` check is sufficient there.
- `V-3` `consumeStream` uses a local `isStale()` predicate that checks both `shouldApplyRunEvent` AND `_streamRunSeq !== mySeq` — both guards are kept for defence in depth.

### Constraints

- Do not remove the existing `shouldApplyRunEvent` check — it is still needed for the run-switch-to-different-run case.
- Do not increment `_streamRunSeq` in `resetRun` or `selectProject` — those already set `runId: undefined`, which makes `shouldApplyRunEvent` return false for any pending stream.

### Open Questions

- `none`

### Source Refs

- `apps/desktop-flowpilot/src/state/store.ts` — `consumeStream`, `openHistoryRun`, `reconnect`
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` — `applyTimelineEvent` F-1 stale detection (unchanged)
- [BUG-074](./BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md) — the earlier fix whose invariant this bug violated

## 1. Issue Summary

Starting a run that requires MCP approval (chat A), switching to another run (chat B), then switching back to chat A causes the approval card to immediately show `decision: "resolved"` — Approve/Deny buttons disappear. The AI hangs indefinitely because the server is still waiting for the approval decision that can no longer be submitted.

**Why this was not triggered before BUG-078:** prior to the sidebar auto-update fix, there was no live status feedback while on another chat. Users naturally waited for a run to finish before navigating away. The original `consumeStream` always exited before any navigation, so concurrent streams for the same `runId` never occurred in practice. BUG-078 (live sidebar polling) made switching away mid-run the natural pattern — start a run, switch to another chat while monitoring progress in the sidebar, switch back — which exposed this latent concurrency issue.

## 2. Parent Links

- impacted coding plan: [CP-18: Refactor Workflow With Session](../../07-Coding-Plan/done/CP-18-Refactor-Workflow-With_Session.md)
- impacted tech design: [SD-09: Approval Gates](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- impacted system spec: [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md)

## 3. Environment and Reproduction

- environment: desktop-flowpilot, any provider with YOLO disabled, any run requiring MCP tool approval
- reproduction steps:
  1. Start a prompt in chat A that triggers an MCP approval card (e.g. Google Drive, Codex MCP).
  2. While the Approve/Deny buttons are visible, click a different run in the Navigator sidebar (chat B).
  3. Click back to chat A in the Navigator.
  4. Observe: the approval card shows resolved state (no buttons); the AI is hanging.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: returning to chat A shows the approval card with Approve/Deny buttons still active; the user can approve and the run continues
- actual: the approval card immediately shows `decision: "resolved"`, Approve/Deny buttons are gone; the AI hangs because the server is still blocked on the approval

## 5. Impact

- users affected: all desktop users who switch between runs while an approval is pending
- workflows affected: any chat or workflow run using tools that require explicit approval
- severity: high — the run is permanently stuck; the user must start a new run to recover

## 6. Root Cause

- hypothesis: BUG-074 F-1 stale detection fires incorrectly on the original `consumeStream` after navigating back to the same run
- confirmed cause: `consumeStream("run-A")` stays alive (blocked in `for await`, waiting for the server) while the user visits chat B. The stale guard `shouldApplyRunEvent(get().runId, runId)` checks `runId` equality. When the user returns to chat A, `get().runId` becomes `"run-A"` again — so the old stream's guard passes. The new replay stream sets `pendingApproval` via `permission_required`. When the old stream then receives any event (triggered by `resumeRun` reconnecting the server session), `applyTimelineEvent`'s F-1 block sees `pendingApproval` set → stamps the card `"resolved"` → clears `pendingApproval`. `approve()` can no longer be called (guard: `if (!pending) return`).
- evidence: code inspection of `consumeStream` (single `shouldApplyRunEvent` guard, no per-stream identity), `applyTimelineEvent` F-1 block, and `openHistoryRun` sequence — confirmed two concurrent `consumeStream` instances for the same `runId` are possible and the runId guard does not distinguish them

## 7. Fix Strategy

- `F-1` Add `_streamRunSeq: number` to store state (initial value `0`). Mirrors the `_historyLoadSeq` pattern from BUG-060 F-3.
- `F-2` Increment `_streamRunSeq` in `openHistoryRun` (inside the `set()` call before `consumeStream`) and in `reconnect` (inside its `set()` call before `consumeStream`). These are the two paths where a new stream starts for a potentially alive existing `runId`.
- `F-3` Update `consumeStream`: capture `mySeq = get()._streamRunSeq` at call time. Replace the inline `shouldApplyRunEvent` check with a local `isStale()` predicate that checks both `shouldApplyRunEvent` AND `get()._streamRunSeq !== mySeq`. When the counter advances, any older stream exits immediately on its next `for await` iteration before applying any event.

## 8. Validation

- `V-1` `npx tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → **TypeScript: No errors found**
- `V-2` Logic invariant verified by inspection: `isStale()` is checked at the top of each `for await` iteration before `applyEvent` is called. An old stream captures `mySeq = N`; after `openHistoryRun` increments `_streamRunSeq` to `N+1`, the old stream's next iteration sees `get()._streamRunSeq (N+1) !== mySeq (N)` → exits. The new replay stream captures `mySeq = N+1` and matches correctly.
- `V-3` End-to-end UI smoke test (start run with approval, switch run, switch back, approve) could not be executed in this session — required on next developer session.

## 9. Regression Guard

- tests: no automated test covers the concurrent-stream scenario; a test would require a mock `AsyncIterable` that can be suspended mid-iteration and two concurrent `openHistoryRun` calls
- alerts: none
- audit checks:
  - `_streamRunSeq` is not reset to `0` in `resetRun` or `selectProject` — it is monotonically increasing across the session to prevent any wrap-around edge case
  - The existing `shouldApplyRunEvent` check is preserved as a secondary guard for the run-switch-to-different-runId case

## 10. Follow-Up Document Updates

- upstream docs that must change: none — the approval gate protocol (SS-08, SD-09) is unchanged; this is a client-side stream lifecycle fix
- notes left unchanged on purpose: BUG-074's F-1 and F-2 logic in `timelineReducer.ts` and `store.ts` are correct and unchanged; the bug was in the stream concurrency model, not the stale detection logic itself
