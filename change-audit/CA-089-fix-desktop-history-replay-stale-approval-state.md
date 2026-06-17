# CA-089: Fix Desktop History Replay Stale Approval State

## Scope

`apps/desktop-flowpilot/src/state/timelineReducer.ts`  
`apps/desktop-flowpilot/src/state/store.ts`  
`apps/desktop-flowpilot/src/state/timelineReducer.test.ts`

Bug: re-opening a completed run from the desktop history panel replays all events from the server. The `permission_required` and `user_question_required` events set `pendingApproval` / `pendingQuestion` in state and render unresolved interaction cards. No event in the replayed stream ever clears them, because the original `approve()` / `answer()` actions cleared that state synchronously on the client — those clearings were never persisted as server events.

Result: every historical run with MCP approvals or user questions showed live Approve/Deny buttons (or an active question form) and the "Action required above before continuing." banner, regardless of the run's completed state. When the user had previously denied an approval, re-opening history wrongly showed "decision: approved" instead of a neutral sentinel.

## Completed

**Layer F-1 — `timelineReducer.ts`: stale detection in `applyTimelineEvent`**

Added a pre-switch detection block at the top of `applyTimelineEvent`:

- If `s.pendingApproval` is set when any follow-up event arrives, the approval was resolved in a prior session. Stamp the matching card with `decision: "resolved"` (neutral sentinel — the actual decision is not persisted in the stream).
- If `s.pendingQuestion` is set when any follow-up event arrives, stamp the matching question card with `answer: "answered"`.
- The `finalize` helper spreads `{ pendingApproval: undefined }` / `{ pendingQuestion: undefined }` when the respective stale flag is set, ensuring every event-handler code path clears them without per-case logic.
- **No type guards** are needed: `...extra` in `finalize` always overrides stale clearing, so a new `permission_required` or `user_question_required` correctly re-sets the pending state even after stale detection ran.

**Layer F-2 — `store.ts` `openHistoryRun()`: post-stream safety net**

After `consumeStream` returns, if `handle.status` (server ground truth) is not `"waiting_approval"` or `"waiting_question"` but `pendingApproval` or `pendingQuestion` is still set, stamp all unresolved cards and clear both fields. This catches runs where the server replay stream ends exactly at the gate event with no persisted follow-up.

**Invariant preserved for live runs:** `approve()` / `answer()` clear `pendingApproval` / `pendingQuestion` synchronously in the same `set()` call before submitting to the server. By the time any server follow-up event arrives, those fields are already `undefined`. Both F-1 stale detection and F-2 post-stream cleanup are therefore no-ops during a live run.

**Sentinel value:** `"resolved"` for approval cards, `"answered"` for question cards. These are intentionally neutral — the server event stream does not persist which decision the user made, so claiming "approved" would be wrong for denied runs.

## Verification

- TypeScript: `tsc --noEmit -p apps/desktop-flowpilot/tsconfig.json` → no errors.
- Unit tests: 14/14 pass (3 pre-existing + 11 new). New tests cover all 5 manual-test use cases:
  - UC1: history replay approved approval → `decision: "resolved"`, buttons hidden
  - UC2: history replay denied approval → `decision: "resolved"` not `"approved"` (BUG-074 regression guard)
  - UC3: history replay answered question → `answer: "answered"`, form hidden
  - UC4: live run approve decision preserved → stale detection is no-op when `pendingApproval` is `undefined`
  - UC5: live run deny decision preserved → `decision: "deny"` not overwritten
  - Additional: consecutive gate stamping, cross-gate (approval after question), YOLO state-machine 3/3, admin-web approval 52/53 (1 pre-existing unrelated failure).
- Logic invariant verified by inspection of `approve()` / `answer()` in `store.ts` and the event replay path (`openHistoryRun` → `consumeStream` → `applyTimelineEvent`).
- End-to-end UI smoke-test (open history with a real completed approval/denial run) performed manually by user — approved and denied cases both confirmed showing `decision: resolved` with no buttons and no banner.

## Residual Notes

- If exact decision values (approve vs deny) need to be surfaced in history, server-side persistence of the decision event in the stream is required. The current `"resolved"` sentinel is correct and honest.
- If a new gate type beyond `permission_required` / `user_question_required` is introduced, add a symmetric stale-detection block in F-1 and a matching branch in the F-2 post-stream cleanup.
