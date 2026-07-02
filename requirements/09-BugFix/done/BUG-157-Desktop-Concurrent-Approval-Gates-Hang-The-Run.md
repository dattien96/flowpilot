## Metadata

- Document ID: `BUG-157`
- Title: `Desktop Concurrent Approval Gates Hang The Run`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates Tech Design](../../06-System-Tech-Design/SD-09-Approval-Gates.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: none
- Related Documents: [BUG-074: Desktop History Replay Leaves Resolved Approvals Pending](../done/BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md), [BUG-105: Desktop History Replay Keeps Approval Gate Open](../done/BUG-105-Desktop-History-Replay-Keeps-Approval-Gate-Open.md), [CA-193: Fix Desktop Concurrent Approval Gates Hang The Run](../../../change-audit/CA-193-fix-desktop-concurrent-approval-gates-hang.md)
- Replaces: none
- Tags: desktop, approval, yolo, concurrency, hang, regression

## AI Quick View

### Summary

- When YOLO is off and a provider turn fans out more than one tool call needing approval at once, the desktop UI shows several "Approval required" cards simultaneously.
- Only the most recently arrived approval could actually be resolved — clicking Approve/Deny on an earlier card either did nothing or silently mis-resolved a different approval, because the client tracked exactly one `pendingApproval` value, not a collection.
- Worse, a stale-approval heuristic added for history-replay support (BUG-074) misfired during live concurrent runs: as soon as a second `permission_required` (or an unrelated `tool_completed`) arrived, it stamped the still-open earlier approval card as `"resolved"` in the UI without ever calling `submitApproval` for it.
- On the server, each approval blocks on its own channel keyed by `approvalId` (`interactive_service.go`); since the client never submitted a decision for the orphaned `approvalId`, that call sat blocked until TTL expiry — hanging the run's turn.

### Current Ask

- Track every outstanding approval as a collection (`pendingApprovals: PendingApproval[]`) instead of a single value, so concurrent tool-call approvals can each be resolved independently by id.
- Narrow the BUG-074 stale-detection heuristic so it only fires on `turn_completed`/`turn_failed` — the one transition that cannot happen in a live run while an approval is genuinely still open — instead of firing on any subsequent event.

### Key Decisions

- `V-1` `pendingApproval?: PendingApproval` becomes `pendingApprovals: PendingApproval[]` in `TimelineState`/`AppState`/`RunSnapshot`. `approve()` takes an explicit `approvalId` and resolves only that entry.
- `V-2` Stale-approval detection (originally BUG-074, for history replay) only clears entries on `turn_completed`/`turn_failed`. A live run cannot end a turn while any tool call is still blocked on approval, so this remains a safe signal that the approval was resolved in a prior session without a captured decision event. A new `permission_required` or an unrelated `tool_completed` while other approvals are pending is normal under concurrency and must not clear anything.
- `V-3` `status` is derived from `pendingApprovals.length > 0` rather than a single flag, so an unrelated event arriving while one approval remains open does not flip status away from `waiting_approval`.
- `V-4` The BUG-105 history-replay-ground-truth cleanup (`settleHistoryReplayPendingState`) now checks each pending approval's own timeline card for an unresolved decision (not just the last timeline item), since several cards can be open at once.
- `V-5` No server-side (`apps/local-runner`) changes were needed: `InteractiveService.RequestApproval`/`SubmitApprovalDecision` already key each approval by its own `approvalId` in an `approvals` map and resolve independently — the hang was purely a client-side orphaning bug.

### Constraints

- Scoped to approvals only, per the reported symptom. `pendingQuestion` (AskUser) has the same single-value shape and could theoretically hit an analogous issue if a provider ever asks two questions concurrently; left unchanged since no concurrent-question path exists today. Flagged under Open Questions.
- GitNexus MCP tools were unavailable in this session; impact analysis was performed via local code inspection (usage-site grep across `store.ts`, `timelineReducer.ts`, `ApprovalCard.tsx`, `ChatInput.tsx`, and the Go `interactive_service.go` approval bridge) instead of `gitnexus_impact`.

### Open Questions

- Should `pendingQuestion` be generalized to `pendingQuestions: PendingQuestion[]` for symmetry, in case a provider ever asks multiple questions concurrently? Not observed in this bug report; left as a follow-up if it surfaces.

### Source Refs

- `apps/desktop-flowpilot/src/state/timelineReducer.ts`
- `apps/desktop-flowpilot/src/state/store.ts` — `approve()`, `settleHistoryReplayPendingState()`, `snapshotRunState()`/`restoreRunSnapshot()`
- `apps/desktop-flowpilot/src/components/ApprovalCard.tsx`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- `apps/local-runner/internal/runner/interactive_service.go` — `RequestApproval`, `SubmitApprovalDecision`
- User screenshot 2026-07-02: three stacked "Approval required" cards, one PowerShell approval stuck and failing to resolve, run hung
- `requirements/09-BugFix/done/BUG-074-Desktop-History-Replay-Leaves-Resolved-Approvals-Pending.md` — the stale-detection invariant this bug breaks under concurrency

## 1. Issue Summary

With YOLO off, a single provider turn can fan out several parallel tool calls that each require approval (e.g. two shell commands requested back-to-back before the user acts on the first). The desktop chat renders one "Approval required" card per tool call, but the Zustand store only ever tracked one `pendingApproval` value. The second `permission_required` event overwrote the first in state, and a stale-approval heuristic (written for history replay, BUG-074) treated the orphaned first approval as "resolved in a prior session" and stamped its card `resolved` in the UI — without ever telling the server. The server-side approval record for that first `approvalId` stayed blocked waiting for a decision that would never arrive, hanging the run.

## 2. Parent Links

- impacted coding plan: none directly — this is a runtime-state bug in the desktop client, not a coding-plan-scoped feature
- impacted tech design: `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md`
- impacted system spec: `requirements/05-System-Specs/SS-08-Approve-Gate.md`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, any provider with YOLO disabled (`step_definitions.yolo_mode` / `workflows.yolo_mode` = false), a turn that issues two or more tool calls each requiring approval before the user responds to the first
- reproduction steps:
  1. Start a chat with YOLO off, in a context where the provider fans out multiple approval-gated tool calls in one turn (e.g. several shell/file operations requested together).
  2. Observe multiple "Approval required" cards stack in the timeline.
  3. Click Approve/Deny on the most recently shown card, then try to act on an earlier card.
  4. Observe: only the latest approval is actually resolvable; earlier cards' buttons either do nothing or the card visually flips to "resolved" on its own without user action, and the run never proceeds — it hangs.
- frequency: reproducible any time more than one approval gate is open at once in a live run

## 4. Expected vs Actual

- expected: every outstanding approval card is independently actionable; clicking Approve/Deny on any card resolves exactly that approval and the run proceeds once all outstanding approvals are decided
- actual: the client tracked a single `pendingApproval`; a second concurrent approval evicted the first from tracking, and a stale-detection heuristic silently marked the evicted card "resolved" in the UI while never submitting a decision to the server — the corresponding server-side approval blocked forever (until TTL expiry), hanging the turn

## 5. Impact

- users affected: all desktop users running with YOLO off whenever a turn issues more than one approval-gated tool call concurrently
- workflows affected: any chat/workflow run using tools that require explicit approval (shell commands, file writes, Google Drive, Codex/Claude MCP approvals) in a provider turn that parallelizes tool calls
- severity: high — the run hangs with no error surfaced to the user; the only escape was waiting for the server's approval TTL to expire or restarting the run

## 6. Root Cause

- hypothesis: the desktop store's approval state was designed under the assumption that only one approval gate can be open at a time in a live run
- confirmed cause:
  - `timelineReducer.ts`'s `pendingApproval?: PendingApproval` (singular) was overwritten by `...extra` on every `permission_required` event (`applyTimelineEvent`, `case "permission_required"`), discarding the previous pending approval's id.
  - The BUG-074 stale-detection block treated "any subsequent event while `pendingApproval` is set" as proof the approval was resolved in a prior session (a history-replay-only invariant) and stamped the orphaned card `decision: "resolved"` — this fired incorrectly on a live second `permission_required` or an unrelated `tool_completed`.
  - `store.ts`'s `approve(decision)` resolved against `get().pendingApproval` (whatever was currently tracked, i.e. the latest), and `ApprovalCard.tsx` never passed its own `approvalId` to `approve()` — so every card's button implicitly targeted the same single tracked value.
  - Server-side, `interactive_service.go`'s `RequestApproval` creates one `approvalRecord` per call keyed by a unique `approvalId` and blocks on `<-rec.resolve`; `SubmitApprovalDecision` resolves strictly by `approvalId` from the `s.approvals` map — the server was already correct for concurrency. The orphaned first approval's `rec.resolve` channel never received a value because the client never called `submitApproval` for that id, so the run hung until `s.approvalTTL` expired.
- evidence:
  - `apps/desktop-flowpilot/src/state/timelineReducer.ts` (pre-fix): `case "permission_required": ... finalize(timeline, { pendingApproval: { approvalId: e.approvalId, details: e.details } })` — always overwrites, never appends
  - `apps/desktop-flowpilot/src/state/store.ts` (pre-fix): `approve(decision) { const pending = get().pendingApproval; ... }` — no `approvalId` parameter
  - `apps/desktop-flowpilot/src/components/ApprovalCard.tsx` (pre-fix): `onClick={() => void approve(d.value)}` — does not pass `approvalId`
  - `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` (pre-fix) had a passing test literally named `"new permission_required while previous pendingApproval is set stamps the first and sets the second"` — the buggy behavior was captured as expected behavior before this fix
  - `apps/local-runner/internal/runner/interactive_service.go:1619-1669` (`RequestApproval`) and `:2746-2780` (`SubmitApprovalDecision`) — confirmed the server already resolves per-`approvalId`, ruling out a server-side fix

## 7. Fix Strategy

- `F-1` `apps/desktop-flowpilot/src/state/timelineReducer.ts`: changed `TimelineState.pendingApproval?: PendingApproval` to `pendingApprovals: PendingApproval[]`. `permission_required` now appends to the array instead of overwriting. Stale-approval detection now only fires on `turn_completed`/`turn_failed` (the only transition that cannot happen live while an approval remains open), clearing every entry and stamping every still-unresolved approval card `"resolved"` — instead of firing on any subsequent event. `status` is derived from `pendingApprovals.length > 0` so status stays `waiting_approval` while any approval remains open, regardless of unrelated events arriving in between.
- `F-2` `apps/desktop-flowpilot/src/state/store.ts`: `approve(approvalId, decision)` now takes an explicit `approvalId` and resolves only the matching entry out of `pendingApprovals`, leaving any other outstanding approvals untouched. `RunSnapshot`/`AppState`/`snapshotRunState`/`restoreRunSnapshot`/`emptyRunSnapshot` updated to the array shape. `settleHistoryReplayPendingState` (BUG-105's ground-truth cleanup) now checks each pending approval's own timeline card for an unresolved decision rather than only the last timeline item, so it correctly handles several cards open at once during history replay.
- `F-3` `apps/desktop-flowpilot/src/components/ApprovalCard.tsx`: passes its own `approvalId` prop into `approve(approvalId, d.value)` so each card resolves itself, not "whatever the store currently thinks is pending."
- `F-4` `apps/desktop-flowpilot/src/components/ChatInput.tsx`: the "Action required above before continuing" banner now checks `pendingApprovals.length > 0 || pendingQuestion` instead of a single truthy value.
- `F-5` No changes needed in `apps/local-runner` (Go) — the server-side approval bridge (`interactive_service.go`) already resolves independently per `approvalId`.

## 8. Validation

- `V-1` TypeScript compilation passes with zero errors (`apps/desktop-flowpilot`: `npx tsc --noEmit -p tsconfig.json`).
- `V-2` `apps/desktop-flowpilot/src/state/timelineReducer.test.ts`: 26/26 tests pass, including 3 new/rewritten regression tests for this bug:
  - `"BUG-157 regression: tool_completed for an unrelated tool does not clear a still-pending approval"` — an unrelated tool finishing no longer orphans a genuinely open approval.
  - `"BUG-157: turn_completed clears every entry when two approvals were concurrently pending"` — both cards get stamped resolved when the turn legitimately ends with approvals outstanding (replay case).
  - `"BUG-157: new permission_required while a previous approval is still pending keeps BOTH open"` — the core regression: a second concurrent approval no longer evicts the first.
  - All prior BUG-074/BUG-111/BUG-116/CP-35 regression tests in this file still pass after adapting them to the array shape.
- `V-3` `apps/desktop-flowpilot/src/state/store.test.ts`: 58/59 tests pass when run via `node --test` with a manual module-alias shim for `@/*`/`@flowpilot/client-core` (this project has no vitest/jest runner wired for these colocated `*.test.ts` files; only `tsc -p tsconfig.phase1-tests.json && node --test .../tests/phase1` is scripted, which does not include `state/*.test.ts`). The 1 failure (`selectProject resets the active chat run when switching projects`) is a pre-existing environment gap (`localStorage is not defined` — no DOM shim in a plain Node harness) unrelated to this fix; the test's actual assertion on `pendingApprovals` was not reached.
- `V-4` End-to-end UI smoke test (two real concurrent tool-call approvals in a live desktop run) could not be executed in this session — requires a live provider session with parallel tool calls; recommended for the next developer session.

## 9. Regression Guard

- tests: `apps/desktop-flowpilot/src/state/timelineReducer.test.ts` (26 tests, 3 new for this bug), `apps/desktop-flowpilot/src/state/store.test.ts` (adapted to the array shape)
- alerts: none
- audit checks:
  - Any new event type added to `ProviderEventDTO` that can legitimately arrive while an approval is outstanding must NOT be added to the stale-detection trigger list (currently only `turn_completed`/`turn_failed`) unless it is proven the turn cannot continue with an approval still open.
  - If a provider is ever changed to issue multiple concurrent `user_question_required` prompts, `pendingQuestion` has the same single-value shape as the pre-fix `pendingApproval` and would need the same array treatment (see Open Questions).

## 10. Follow-Up Document Updates

- upstream docs that must change: none — SS-08 and SD-09 describe the approval-gate protocol at the server/event level, which already supported concurrency; the client-side ephemeral state model is an implementation detail not documented there (same conclusion as BUG-074).
- notes left unchanged on purpose: `pendingQuestion` remains a single value; no concurrent-question code path exists today (see Open Questions).
