---
Document ID: BUG-074
Title: Desktop History Replay Leaves Resolved Approvals in Pending State
Phase: bugfix
Status: done
Owner: FlowPilot
Reviewers: FlowPilot
Created: 2026-06-17
Last Updated: 2026-06-17
Parent Documents: requirements/05-System-Specs/SS-08-Approve-Gate.md, requirements/06-System-Tech-Design/SD-09-Approval-Gates.md
Child Documents: none
Related Documents: requirements/09-BugFix/done/BUG-039-Google-Drive-Approval-Replay-Loses-Step-Scoped-Process-Key.md, requirements/09-BugFix/done/BUG-046-Desktop-History-Replay-Loses-User-Prompts.md, change-audit/CA-089-fix-desktop-history-replay-stale-approval-state.md
Replaces: none
Tags: desktop, approval, history-replay, timeline-reducer, regression
---

## AI Quick View

### Summary

- When the user re-opens a completed run from the desktop history panel, all events are replayed from offset 0 via `streamRun`.
- The `permission_required` event replayed from history correctly pushes an approval card onto the timeline, but it also sets `pendingApproval` in store state and leaves the card's `decision` field `undefined`.
- The original `approve()` action that resolved the approval during the live session cleared `pendingApproval` synchronously on the client — it did not emit a persistent event — so no resolution signal exists in the replayed stream.
- As a result, re-opened history shows Approve/Deny buttons on already-resolved approval cards and incorrectly renders the "Action required above before continuing." banner in the chat input.

### Current Ask

- Fix `applyTimelineEvent` so that a replayed `permission_required` event followed by any subsequent event is automatically resolved in the client timeline, matching the state of the original run.

### Key Decisions

- `V-1` The invariant "if any event arrives after a `permission_required` and `pendingApproval` is still set, the approval was resolved in a prior session" holds because `approve()` clears `pendingApproval` synchronously before submitting, so in a live run no follow-up event can arrive while `pendingApproval` is set.
- `V-2` Fix touches `timelineReducer.ts` (F-1 reducer path) and `store.ts` (F-2 post-stream safety net). No changes to the event protocol, server, or `ApprovalCard`.
- `V-3` The sentinel decision string for replay is `"resolved"` (neutral — does not claim approve or deny, since the actual decision is not persisted in the event stream).
- `V-4` No type guards are needed in F-1: `...extra` in `finalize` always overrides stale clearing, so a new `permission_required` correctly re-sets `pendingApproval` even after stale detection cleared it.
- `V-5` `pendingQuestion` has the same structural gap and is covered by a symmetric implementation in the same fix (both F-1 and F-2 layers).

### Constraints

- Do not modify the server-side event protocol or add new event types.
- Keep the fix idempotent: in live runs `pendingApproval` is already `undefined` when follow-up events arrive, so the new code path is never exercised.
- GitNexus tools were unavailable in this session; impact analysis was performed via local code inspection instead.

### Open Questions

- None. The `pendingQuestion` structural gap (replayed `user_question_required` leaving question cards interactive) was addressed in the same fix iteration via symmetric stale detection in F-1 and F-2.

### Source Refs

- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/store.ts` — `openHistoryRun()`, `approve()`
- `apps/desktop-flowpilot/src/components/ApprovalCard.tsx`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx:676`
- User screenshot 2026-06-17: re-opened history showing live Approve/Deny buttons and "Action required" banner on a resolved MCP approval

## 1. Issue Summary

Re-opening a completed run from the desktop history panel replays all events from the server. The `permission_required` event sets `pendingApproval` in the Zustand store and adds an approval card with `decision: undefined`. In the original live session, the user clicked Approve and `approve()` cleared `pendingApproval` synchronously before submitting to the server — that clearing was never serialised as an event. During replay there is no matching resolution event, so:

1. The approval card renders Approve/Deny buttons (ApprovalCard shows buttons when `decision === undefined`).
2. The "Action required above before continuing." banner in ChatInput renders because `pendingApproval` is non-null.

## 2. Parent Links

- impacted coding plan: none directly; touches the timeline reducer introduced for the desktop chat/session flow
- impacted tech design: `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- impacted system spec: `requirements/05-System-Specs/SS-08-Approve-Gate.md`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, any provider (Claude, Codex, Gemini) with YOLO disabled so MCP tool calls produce real approval cards
- reproduction steps:
  1. Start a chat with a provider that triggers an MCP approval (e.g. Google Drive search).
  2. Click Approve when the card appears. The run completes.
  3. Open the History panel and click the completed run.
  4. Observe: the approval card shows Approve/Deny buttons and the bottom bar shows "Action required above before continuing."
- frequency: 100% reproducible on any run with at least one resolved approval

## 4. Expected vs Actual

- expected: re-opened history shows resolved approval cards in their approved state (no buttons, decision badge visible); "Action required" banner is hidden
- actual: resolved approval cards re-render Approve/Deny buttons; "Action required" banner is incorrectly shown; clicking Approve on a historical card would submit a duplicate approval request to the server

## 5. Impact

- users affected: all desktop users who re-open historical runs containing MCP approval events
- workflows affected: any chat run using tools that require explicit approval (Google Drive, Codex MCP, etc.)
- severity: medium — misleading UI; does not corrupt the underlying run, but a confused user clicking Approve on a historical card would submit an unexpected approval request

## 6. Root Cause

- hypothesis: the client-side `approve()` action clears `pendingApproval` synchronously, but this clearing is not recorded as an event in the server-side stream
- confirmed cause: `applyTimelineEvent` sets `pendingApproval` when `permission_required` fires, and no other event in the replayed stream ever clears it. The resolution was ephemeral client state, not a persisted event.
- evidence:
  - `store.ts:approve()` (line 438–448): clears `pendingApproval` and stamps `decision` on the card in one synchronous `set()` call, then POSTs to the server — the clearing happens client-side only
  - `timelineReducer.ts:applyTimelineEvent`: only the `permission_required` case writes to `pendingApproval`; no other case clears it
  - `openHistoryRun()` (line 510–533): clears `pendingApproval: undefined` before replay starts, then `consumeStream` replays all events from seq 0, re-setting `pendingApproval` when `permission_required` is encountered
  - `ChatInput.tsx:676`: banner renders on `pendingApproval || pendingQuestion` with no run-status guard

## 7. Fix Strategy

Two complementary layers were applied to cover all replay paths:

- `F-1` **Reducer-level stale detection (`timelineReducer.ts`)**: at the top of `applyTimelineEvent`, before the event switch, detect a stale pending approval: if `s.pendingApproval` is set when any follow-up event arrives, the approval was resolved in a prior session. Stamp the matching approval card with `decision: "resolved"` (neutral sentinel — the actual decision is not persisted in the stream) and record the stale flag. No type guards are needed: when a new `permission_required` fires it sets `pendingApproval` via `...extra` in `finalize`, which always overrides the stale clearing. The `finalize` helper spreads `{ pendingApproval: undefined }` whenever the stale flag is set, so every code path clears `pendingApproval` without per-case logic. This covers runs where the server event log includes `tool_completed` or `turn_completed` after the approval.
- `F-2` **Post-stream cleanup (`store.ts` `openHistoryRun`)**: after `consumeStream` returns, if `handle.status` (server ground truth) is not `"waiting_approval"` or `"waiting_question"` but `pendingApproval` is still set, stamp all unresolved approval cards and clear `pendingApproval`/`pendingQuestion`. This is the safety-net layer for runs where the server replay stream ends after `permission_required` with no persisted follow-up event.
- `F-3` **Unit tests (`timelineReducer.test.ts`)**: added 11 tests covering all 5 manual-test use cases plus regression guards. Approval: `permission_required` setup, `tool_completed` stamps resolved, `turn_completed` stamps resolved, live approve preserved, live deny preserved, consecutive gates, denied-card stamps resolved not approved (explicit BUG-074 regression). Question: `user_question_required` setup, follow-up stamps answered, cross-gate (`permission_required` after question stamps question), live question preserved.

## 8. Validation

- `V-1` TypeScript compilation passes with zero errors (local tsc `--noEmit`).
- `V-2` All 14 unit tests pass: 3 pre-existing tests (`tool_completed` thinking row, `turn_completed` thinking removal, `shouldApplyRunEvent`) plus 11 new tests covering all 5 manual-test use cases, approval and question stale detection, live approve/deny preservation, consecutive gates, and the BUG-074 regression guard (denied card stamps "resolved" not "approved").
- `V-2b` YOLO state-machine tests: 3/3 pass (YOLO disabled pauses, YOLO enabled auto-approves, rejected step retry). No regression in server-side approval flow.
- `V-2c` Admin-web approval tests: 52/53 pass. The 1 failure (`submit-google-drive-write-approval-http-handler > form submission`) is pre-existing and unrelated to BUG-074 — our branch does not touch that file.
- `V-3` Logic invariant verified by inspection: in a live run `approve()` clears `pendingApproval` synchronously before the server sends any follow-up event; `s.pendingApproval` is `undefined` when the next event arrives, so both F-1 stale detection and F-2 post-stream cleanup are no-ops.
- `V-4` End-to-end UI smoke-test (re-open a real history run with MCP approval events) could not be executed in this session — required on next developer session.

## 9. Regression Guard

- tests: 11 new tests in `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` — 14 total — cover approval replay, question replay, live approve/deny preservation, consecutive gates, and the BUG-074 regression guard
- alerts: none
- audit checks:
  - No type guards are used in F-1 stale detection. The `...extra` spread in `finalize` ensures a new `permission_required` or `user_question_required` always wins over the stale clearing — no per-event exclusions needed.
  - The post-stream cleanup in F-2 (`store.ts`) stamps ALL unresolved cards `decision: "resolved"` / `answer: "answered"` when the run is complete — neutral sentinels. If exact decision values (approve vs deny) need to be surfaced in history, add server-side persistence of the decision event.
  - If a new gate type is introduced beyond `permission_required` / `user_question_required`, add a symmetric stale-detection block and a post-stream cleanup branch for it.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — the fix brings client behaviour back into alignment with existing approval-gate semantics defined in SS-08 and SD-09
- notes left unchanged on purpose:
  - SD-09 and SS-08 describe approval gates from a server/protocol perspective; the client-side ephemeral state model is an implementation detail not documented there
