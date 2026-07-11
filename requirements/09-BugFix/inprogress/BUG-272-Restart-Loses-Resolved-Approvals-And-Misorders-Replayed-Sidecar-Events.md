# BUG-272: Restart Loses Resolved Approvals And Mis-Orders Replayed Sidecar Events

## Metadata

- Document ID: `BUG-272`
- Title: `Restart Loses Resolved Approvals And Mis-Orders Replayed Sidecar Events`
- Phase: `bugfix`
- Status: `in_progress` — code + automated tests complete 2026-07-10; live UI re-verification against a real server restart still pending (mirrors BUG-271's own land-then-live-reverify trajectory).
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [BUG-271: Resolved Question Vanishes Or Reappears Interactive After Server Restart](../done/BUG-271-Resolved-Question-Vanishes-Or-Reappears-Interactive-After-Server-Restart.md)
- Child Documents: `none`
- Related Documents: [Task-212: Grok Parity Hardening And Live DOD](../../08-Task/done/Task-212-Grok-Parity-Hardening-And-Live-DOD.md) (the sibling Grok transcript-replay gap, DOD-5/DOD-9, closed in the same session)
- Replaces: `none`
- Tags: `agent-flow-engine`, `resume`, `approval`, `regression`, `restart`

## AI Quick View

### Summary

- BUG-271 made a resolved **question** survive a full server restart by persisting the raw `user_question_required` event to the CP-41 flow-events sidecar and its resolution to `questions.ndjson`, then merging them in `reconstructRun`.
- Live re-testing that fix surfaced two remaining defects in the same restart-replay path:
  - **Approvals were never given the same treatment.** `permission_required` was not a sidecar event type and `localFileSessionStore` had no `UpsertApproval` write-through — so after a full restart a resolved approval card vanished entirely (only the question survived). This is the "Claude miss view resolve của approve tool" report.
  - **The BUG-271 sidecar events rendered pinned above the entire prior conversation.** `reconstructRun` seeds sidecar/question events into `rs.events` *before* `seedTranscriptFromDisk` loads the real transcript (they are two separate calls), so the sidecar events always claimed the lowest `Seq` numbers and the client (which renders in `Seq` order) drew them first regardless of when they happened.

### Current Ask

- Make a resolved approval survive a full restart read-only (the approval-side twin of BUG-271), and stop restored sidecar events from jumping to the top of the timeline — without weakening any prior restart/resume fix (BUG-060/074/080/118/121/157/174/233/242/248/271).

### Key Decisions

- `V-1` Mirror BUG-271's three-piece question mechanism exactly for approvals: add `EventPermissionRequired` to `isFlowSidecarEventType`; add `localFileSessionStore.UpsertApproval`/`ListApprovalsByRun` writing through to `approvals.ndjson`; add an `ApprovalHistoryReader` interface; merge the recorded `Decision` (or drop expired) onto the replayed event in `reconstructRun`.
- `V-2` Client renders a replayed approval read-only via a new `decision` field on the `permission_required` DTO (the twin of the question `answer` field): a decision-carrying event must not re-enter `pendingApprovals` nor flip run status to `waiting_approval`.
- `V-3` Fix ordering by moving the sidecar prefix to the end: `reconstructRun` records how many leading `rs.events` are sidecar-origin (`sidecarPrefixCount`); `seedTranscriptFromDisk` moves that prefix after the replayed transcript and renumbers `Seq` gap-free. Not perfect chronological interleaving (the transcript loader keeps no per-turn timestamps), but "after the conversation" is uniformly closer to correct than "always first", and matches what these events are — durable current state, not history.

### Constraints

- Additive to the restart-replay path only; must not change live-turn behavior or any other provider's resume.
- No full rewrite of `approvals.ndjson` — append-only write-through, last-write-wins per `ApprovalID` on load, mirroring `questions.ndjson`.

### Open Questions

- `Q-1` Exact chronological interleaving of sidecar events with transcript turns is not recoverable without per-turn timestamps in the transcript loader; deferred as a cosmetic refinement.

### Source Refs

- Live report (2026-07-10): after restart, Claude showed history but the resolved approval card was missing (only the question showed); the surviving question also rendered above the whole conversation.
- `apps/local-runner/internal/runner/local_file_session_store.go` (`isFlowSidecarEventType`, `UpsertApproval`/`ListApprovalsByRun`/`loadApprovalsFromDisk`), `workflow_store.go` (`ApprovalHistoryReader`, `fakeWorkflowStore.ListApprovalsByRun`), `interactive_resume.go` (`reconstructRun` approval merge + `sidecarPrefixCount`, `reorderSidecarPrefixToEnd`), `provider_event.go` (`Decision` field).
- `apps/desktop-flowpilot/src/state/timelineReducer.ts` (`permission_required` case + `statusFromEvent`), `types/contract.ts` (`decision` on the DTO).

## 1. Issue Summary

After a full FlowPilot server restart, reopening a Claude/Codex chat that had a resolved approval showed the conversation but **not** the resolved approval card — only a resolved question card survived. Separately, the surviving question (and any other restored sidecar event) rendered pinned at the very top of the transcript instead of at its real position.

## 2. Parent Links

- impacted coding plan: `CP-41` (flow-events sidecar), `CP-44` (context source registry / resume path)
- impacted tech design: `SD-05` (Workflow Engine resume)
- impacted system spec: `SS-08` (Approve Gate)

## 3. Environment and Reproduction

- environment: FlowPilot desktop + local runner, offline `localFileSessionStore` backend (`<cwd>/.flowpilot/chats`).
- reproduction steps: (1) run a Claude chat that triggers a tool approval; approve it. (2) fully restart the runner process (not just an SSE reconnect). (3) reopen the chat.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: the resolved approval card replays read-only (showing the recorded approve/deny) at its position in the conversation, exactly like a resolved question.
- actual: the approval card was gone entirely; the resolved question card that did survive was drawn above the whole prior conversation.

## 5. Impact

- users affected: anyone reopening a Claude/Codex chat with a prior approval after a runner restart.
- workflows affected: chat history review / audit of what was approved.
- severity: medium — no data loss (the provider transcript is intact), but the approval audit trail and timeline ordering were visibly wrong after restart.

## 6. Root Cause

- confirmed cause (approvals): `permission_required` was never persisted for restart — it was not in `isFlowSidecarEventType`, and `localFileSessionStore` had no `UpsertApproval` override (it delegated to the in-memory `fakeWorkflowStore`), so both the raw event and its resolution were lost on restart. The provider transcript has `tool_use`/`tool_result` but no FlowPilot approval-card concept, so nothing reconstructed it. This is exactly the BUG-271 question defect, left unfixed on the approval side.
- confirmed cause (ordering): `reconstructRun` runs before `seedTranscriptFromDisk` and necessarily assigns its sidecar/question events the lowest `Seq` values; the client renders strictly in `Seq` order and does not reorder, so the sidecar prefix always drew first.

## 7. Fix Strategy

- `F-1` Persist approvals across restart: `EventPermissionRequired` added to `isFlowSidecarEventType`; `localFileSessionStore.UpsertApproval` writes through to `approvals.ndjson` and `loadApprovalsFromDisk` reads it back at startup; `ListApprovalsByRun` + `ApprovalHistoryReader` expose it.
- `F-2` Merge approval resolution in `reconstructRun`: stamp `Decision` onto the replayed `permission_required` event when resolved (fallback `"resolved"` when only `Status` was recorded); drop expired approvals. New `Decision` field on `ProviderEvent`/DTO.
- `F-3` Frontend read-only replay: `permission_required` with `decision` set renders read-only, stays out of `pendingApprovals`, and does not flip status to `waiting_approval` (mirrors the question `answer` path).
- `F-4` Ordering: `reconstructRun` sets `sidecarPrefixCount`; `seedTranscriptFromDisk` calls `reorderSidecarPrefixToEnd` to move the prefix after the replayed transcript and renumber `Seq`.
- `F-6` **Backfill old chats (found in live re-test 2026-07-10, after F-5).** F-5 only helps approvals resolved by the fixed build; a chat whose approval was resolved by a pre-fix build still has no `approvals.ndjson` record, so on reopen it still replayed interactive. Since `reconstructRun` is exclusively the after-restart path, a record-less `permission_required` can never be live-actionable again — so it is now stamped a generic read-only `"resolved"` (the concrete approve/deny is unrecoverable, never having been persisted). This makes pre-fix chats render read-only on reopen instead of a broken Approve/Deny prompt. New resolutions still carry the exact decision via F-5.
- `F-5` **Persist on the live approve path (found in live re-test 2026-07-10, after F-1..F-4 landed).** The first pass added the read side (sidecar + `approvals.ndjson` reader + reconstruct merge) but the LIVE interactive approve path (`submitApprovalDecision`) still resolved the record in memory only — it never called `persistApproval`, unlike `AnswerQuestion` which persists via `persistQuestion`. So a genuinely user-approved action left no resolved record on disk, and after a restart the card still replayed interactive and flipped the run to `waiting_approval` (the reported symptom, for both Claude and Grok). Fixed by building a `ProviderApprovalState` snapshot in `submitApprovalDecision` and calling `persistApproval` outside the lock — the exact analog of the question path. F-1..F-4's unit tests seeded `approvals.ndjson` directly, which is why they passed while the live path silently didn't persist; `V-7` now drives the real path.

## 8. Validation

- `V-1` `TestReconstructRunStampsDecisionOnRestoredApprovalEventAfterFullRestart` — resolved approval survives with `Decision` stamped.
- `V-2` `TestReconstructRunDropsExpiredApprovalEventAfterFullRestart` — expired approval is dropped.
- `V-3` `TestLocalFileSessionStoreApprovalsSurviveRestart` — `approvals.ndjson` round-trips through a fresh store instance.
- `V-4` `TestSeedTranscriptFromDiskMovesSidecarPrefixAfterReplayedTranscript` — sidecar prefix moves to the end with gap-free `Seq`.
- `V-5` Frontend `timelineReducer.test.ts`: `BUG-ApprovalReplay-Restart: replay of an already-resolved approval renders read-only and does not re-enter pendingApprovals`.
- `V-7` `TestSubmitApprovalDecisionPersistsResolvedApprovalForRestart` — drives the real `SubmitApprovalDecision` path (not a hand-seeded file) and proves a second store instance (a restart) reads the resolved decision back. Guards F-5.
- `V-8` `TestReconstructRunBackfillsRecordlessApprovalAsReadOnlyAfterRestart` — a permission_required with no `approvals.ndjson` record is stamped read-only (Decision set) on restart, not left interactive. Guards F-6 (old-chat backfill).
- `V-6` **Pending (live):** real restart of a Claude/Grok chat with a prior approval, confirming the card replays read-only at the right position and the run does not flip to `waiting_approval`.

## 9. Regression Guard

- tests: the five automated tests above; full `go test ./internal/runner/...` green except the pre-existing environment-dependent failures unrelated to this fix; frontend `timelineReducer.test.ts` green.
- audit checks: BUG-271's own question tests remain green (shared `reconstructRun`/sidecar path).

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is a defect in the BUG-271 mechanism, documented here.
- notes left unchanged on purpose: exact chronological interleaving of sidecar events (`Q-1`) is deferred as cosmetic.
