# BUG-233: Flow Step Timeline Shows Stale "All Done" While A Reviewer Still Runs; Blocked Card Shows Internal Diagnostic Instead Of Reviewer Findings

## Metadata

- Document ID: `BUG-233`
- Title: `Flow Step Timeline Shows Stale "All Done" While A Reviewer Still Runs; Blocked Card Shows Internal Diagnostic Instead Of Reviewer Findings`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [BUG-231: Escalate/Awaiting-User Flow State Is Not Actionable](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md), [CA-226: Hub Synthesis Fallback Escalation](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `none`
- Related Documents: [CA-235: Flow Step Sync Settlement + Reviewer-Findings Fallback](../../../change-audit/CA-235-flow-step-sync-settlement-and-reviewer-findings-fallback.md)
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, step-timeline, desktop, flow-control, escalate`

## AI Quick View

### Summary

- Surfaced by the same live test session that shipped BUG-231's Continue/Stop card. Two separate, pre-existing issues — neither caused by BUG-231's own code:
  1. **Step timeline staleness**: right after a Continue-driven new round starts (or a reviewer cohort joins), the step timeline briefly (and sometimes not-so-briefly) shows every step as done while a freshly spawned reviewer child agent is still visibly `running` in the agent panel.
  2. **Blocked-card content mismatch**: the new BUG-231 `FlowAwaitingUserCard` sometimes shows an internal diagnostic string ("Hub synthesis turn completed without calling submit_review_outcome. Final message: …") instead of anything resembling the reviewers' actual findings — confusing to a user who is being asked to make a call.
- Both fixed. Root cause for #1 turned out to be entirely server-side (a Go concurrency ordering bug), not the client-side dual-refresh race originally suspected — no desktop changes were needed.

### Current Ask

- Implemented (`D-1`..`D-3`, DoD §11 items 1-6/DOC all done). Both bugs fixed in `apps/local-runner/internal/runner`, with regression tests.

### Key Decisions

- `D-1` (unchanged from investigation) For bug #4, the fallback shows a **concise, consolidated summary of the reviewers' actual findings** — not the raw internal diagnostic sentence.
- `D-2` **(Resolved, supersedes the original two-option framing)** Bug #1's root cause is server-side, not client-side: every flow-control code path that settles a step transition (`applyFlowControl`'s "looping"/"awaiting_user"/"escalate" branches, `resumeFlowWithFeedback`, the cohort-join reviewer-DONE/hub-RUNNING write) dispatched its `setFlowStepStatus` write inside a `go func(){...}()`, then called `emitAgentGraph`/returned — and `emitAgentGraph` is exactly what fires the SSE `agent_graph_updated` event the desktop reacts to by refreshing the step-runtime snapshot. There was no client-side ordering to fix; the fix is to make every one of those writes run synchronously, before the graph is emitted, so the backend's `LoadRunSteps()` can never be read mid-transition. `workflowStore.ApplyStepTransition` uses its own independent lock, so doing this synchronously (including inside `emitLocked`, which already holds `InteractiveService.mu`) does not risk deadlock.
- `D-3` For bug #4, the joined reviewer-cohort note (`buildCohortNote`'s output) is retained on the run (`interactiveRun.lastCohortNote`) past `pendingAgentContext` being drained into the hub's synthesis turn, so the CA-226 fallback can still read it after that turn completes. A new helper, `summarizeCohortNoteForUser`, strips the note's engine-internal header/instructions and returns only the per-reviewer findings lines for the card. Also fixed in the same code path: the CA-226 fallback was additionally marking the hub node `FAILED` right after `applyFlowControl(escalate)` had already (correctly, per BUG-231) settled it to `WAITING_USER_APPROVAL` — an oversight from before BUG-231 existed that silently overwrote BUG-231's own fix for this exact scenario. Removed.

### Open Questions

- None remaining. (`Q-1`/`Q-2` from the original investigation are resolved by `D-2`/`D-3` above.)

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` `applyFlowControl` — "looping"/"awaiting_user" branches (the `continue` case) and the "escalate" case: step-status writes moved from `go func(){...}()` to synchronous, before `emitAgentGraph`.
- `apps/local-runner/internal/runner/interactive_service.go` `resumeFlowWithFeedback`: hub-node `RUNNING` transition moved from `go s.setFlowStepStatus(...)` to synchronous, before `emitAgentGraph`.
- `apps/local-runner/internal/runner/interactive_service.go` `emitLocked`'s `EventTurnCompleted` cohort-join branch: reviewer-DONE/hub-RUNNING writes moved out of the `go func(){...}()` (which also called `maybeAutoReinvokeHubWithNote`) to run synchronously before that goroutine is spawned.
- `apps/local-runner/internal/runner/interactive_service.go` — new `interactiveRun.lastCohortNote` field, `lastCohortNoteFor(runID)` getter, `summarizeCohortNoteForUser(note)` helper; CA-226 fallback in the hub-turn-completion handler now builds `Summary` from the reviewers' findings when available, and no longer overrides the hub node to `StepStatusFailed`.

## 1. Issue Summary

Two independent, pre-existing defects in the flow-engine review loop, surfaced by live testing of BUG-231's new awaiting-user UI:

1. The step timeline can show a stale "all steps done" snapshot immediately after a new review round starts or a reviewer cohort joins, while a reviewer/hub node is still actually transitioning — reading as a false completion.
2. The awaiting-user card can display an internal engine diagnostic sentence instead of anything resembling the reviewers' findings, when the loop blocks via the CA-226 no-outcome-call fallback path rather than a genuine `submit_review_outcome(blocked)` call.

## 2. Parent Links

Surfaced during live validation of [BUG-231](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md); the diagnostic text bug #4 originates in [CA-226](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md)'s fallback escalation.

## 3. Environment and Reproduction

- Built-in "Review Loop" workflow, flow-engine-driven run, 2 reviewers (mixed verdicts: one approve, one changes-requested).
- Bug #1: reproduced once during live testing right after pressing Continue on a blocked loop, which spawned a fresh reviewer cohort for another round — the step-timeline panel briefly showed every step (including the still-running reviewer's) as done.
- Bug #4: reproduced once during live testing where the hub's synthesis turn completed without calling `submit_review_outcome`, triggering the CA-226 fallback; the awaiting-user card showed the fallback's diagnostic `Summary` text.
- No automated repro reproduced the exact timing window for #1 (that would require an artificial delay); instead, the fix is verified by asserting the correct step status is visible **immediately** after the relevant call returns — the invariant a genuine fix must establish — since the whole point of a synchronization bug is that no fixed sleep reliably rules it out.

## 4. Expected vs Actual

- Bug #1 — Expected: the step timeline reflects the true in-flight state (a running reviewer shows as running) at all times, including immediately after a new round starts. Actual: a stale "all done"/prior-round snapshot could render transiently (or longer, under load) while a reviewer or hub node was genuinely mid-transition.
- Bug #4 — Expected: when asked to make a call on a blocked loop, the user sees content that helps them decide (what the reviewers actually found), and the step timeline shows the hub waiting for them, not failed. Actual: the user could see an internal engine sentence about the hub's tool-calling behavior (no decision-relevant information), and the hub node showed `FAILED`.

## 5. Impact

- Bug #1: confusing/misleading status display; could cause a user to believe a run finished and navigate away while a reviewer is still consuming provider budget.
- Bug #4: undermined the very purpose of BUG-231's Continue/Stop card — the user was asked to decide but shown no basis for the decision, and the step timeline read as a hard failure instead of an awaiting-user pause.

## 6. Root Cause

- **Bug #1 (confirmed server-side, not client-side)**: Every flow-control code path that transitions a step's status on the CA-226/BUG-231/BUG-233-relevant round-boundary events (new round starting, cap/escalate block, Continue resuming, cohort join) wrote that transition inside a `go func(){...}()`, then called `s.emitAgentGraph(...)` (or returned to a caller that does) without waiting for the goroutine. `emitAgentGraph` fires the SSE `agent_graph_updated` event that the desktop reacts to by refreshing both the agent panel and the step-runtime snapshot (`refreshAgentRuns` / `refreshWorkflowStepRuntime` in `store.ts`) — so the desktop's HTTP request to `LoadRunSteps()` could arrive and be served **before** the just-spawned goroutine's write landed, returning the previous round's/previous status's step snapshot. The two-refresh-channel race originally suspected on the desktop side was a red herring: with all four write sites made synchronous (this fix), there is no window left for a stale read regardless of how the desktop sequences its two refreshes.
- **Bug #4**: `applyFlowControl`'s CA-226 fallback constructed `Summary` as a hard-coded diagnostic sentence with the hub's raw final message appended, and this `Summary` ended up in `AgentLoopState.GateReason`, which `FlowAwaitingUserCard` displays verbatim. The joined reviewer-cohort text (`buildCohortNote`'s output) lived only transiently in `pendingAgentContext`, which is drained into the hub's prompt when its synthesis turn starts — by the time that turn completes and the fallback fires, the note was already gone. Separately, the fallback's step-timeline handling was stale itself: it unconditionally set the hub node to `StepStatusFailed` immediately after `applyFlowControl(escalate)` had already settled it to `WAITING_USER_APPROVAL` per BUG-231's contract, silently reverting that fix for this exact fallback scenario (this line predates BUG-231 and was never updated when BUG-231 changed what "escalate" is supposed to settle the hub node to).

## 7. Fix Strategy

- Bug #1: make every step-status write on these paths synchronous, executed before the corresponding `emitAgentGraph` call (or before spawning any subsequent goroutine that itself calls it). `workflowStore.ApplyStepTransition` (the underlying write) uses its own lock (`fakeWorkflowStore.mu` / the Supabase store's own transaction), independent of `InteractiveService.mu`, so this is safe to do even from inside `emitLocked`, which already holds `InteractiveService.mu` — no deadlock risk, and these are cheap in-memory (or single-row) writes, not network calls that would justify async dispatch.
- Bug #4: capture the joined cohort note onto `interactiveRun.lastCohortNote` at the moment it's built (both the `EventTurnCompleted` and `EventTurnFailed` cohort-join sites), read it back via `lastCohortNoteFor(runID)` in the CA-226 fallback, and reduce it to a human-facing findings list with `summarizeCohortNoteForUser` (strips the `[flow-engine joined result note]` header, the `Flow round N` line, and everything from `---`/`Synthesize:` onward — the parts written for the hub's own prompt, not a human). Fall back to the pre-existing diagnostic sentence only if no cohort note is available (e.g. escalate with no prior cohort join). Also removed the stale post-fallback `StepStatusFailed` override so the hub node keeps BUG-231's `WAITING_USER_APPROVAL` settlement.

## 8. Validation

- `go build ./...` / `go vet ./internal/runner/...` — clean.
- `go test ./internal/runner/...` — 1068 passed, 15 failed (identical pre-existing baseline established in BUG-231/CA-233 — Windows-path/Codex-CLI/Google-Drive environment gaps, unrelated to this change), 14 skipped.
- New/updated tests, all passing, including a 10x `-count=10` stress run (200/200) to rule out flakiness in the newly-synchronous assertions:
  - `TestApplyFlowControlEscalateSettlesHubToWaitingUser` / `TestApplyFlowControlCapReachedSettlesHubToWaitingUser` — tightened from a `waitLoop` (eventual) assertion to an immediate one: the hub node is `WAITING_USER_APPROVAL` the instant `applyFlowControl` returns.
  - `TestApplyFlowControlLoopingResetsStepsSynchronously` (new) — a new round's entry-node `RUNNING` and downstream-node `PENDING` transitions are visible immediately after `applyFlowControl("continue")` returns.
  - `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap`'s "cap reason auto-extends" sub-test — added an immediate hub-node-`RUNNING` assertion.
  - `TestE2EReviewLoopSynthesisFallbackEscalates` — updated to assert the new behavior: `GateReason` contains the reviewers' labels/findings (not the diagnostic sentence), and the hub node settles to `WAITING_USER_APPROVAL` (not `FAILED`).
  - `TestSummarizeCohortNoteForUserStripsEngineInstructions` / `TestSummarizeCohortNoteForUserEmptyInputReturnsEmpty` (new, in `agent_orchestrator_test.go`).
- Not executed: a live click-through in the running desktop app (same sandbox limitation as CA-233/CA-234 — no reachable local-runner backend). No desktop code changed for this fix (bug #1's fix is entirely server-side; bug #4 reuses the existing `GateReason` → `FlowAwaitingUserCard` plumbing from BUG-231 unchanged), so the existing desktop test coverage for that plumbing (`store.test.ts`) still applies unmodified.

## 9. Regression Guard

- Bug #1: the four synchronicity assertions listed under Validation above — each targets a specific write site (continue/looping, continue/cap-reached, escalate, resumeFlowWithFeedback) and would fail again if any of them regressed back to a `go func(){...}()` dispatch before the corresponding graph emission.
- Bug #4: `TestE2EReviewLoopSynthesisFallbackEscalates` (content + step status) and the two `summarizeCohortNoteForUser` unit tests.

## 10. Follow-Up Document Updates

- None required — this fix is self-contained to `apps/local-runner/internal/runner`; no contract/schema changes reached the desktop.

## 11. Definition of Done

- `[x]` **DOD-1 — Bug #1 fix.** All four step-status write sites implicated in the race (`applyFlowControl` looping/awaiting_user/escalate branches, `resumeFlowWithFeedback`, cohort-join reviewer-DONE/hub-RUNNING) made synchronous, before their respective `emitAgentGraph` call.
- `[x]` **DOD-2 — Bug #1 regression test.** `TestApplyFlowControlEscalateSettlesHubToWaitingUser`, `TestApplyFlowControlCapReachedSettlesHubToWaitingUser` (tightened), `TestApplyFlowControlLoopingResetsStepsSynchronously` (new), `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap` (extended) — all assert the step status immediately after the call returns, with no wait.
- `[x]` **DOD-3 — Bug #4 fix.** `lastCohortNote` capture + `summarizeCohortNoteForUser` reduction wired into the CA-226 fallback's `Summary`; stale `StepStatusFailed` override removed.
- `[x]` **DOD-4 — Bug #4 regression test.** `TestE2EReviewLoopSynthesisFallbackEscalates` (updated), `TestSummarizeCohortNoteForUserStripsEngineInstructions`, `TestSummarizeCohortNoteForUserEmptyInputReturnsEmpty` (new).
- `[x]` **DOD-5 — Full regression green.** `go build`/`go vet`/`go test ./internal/runner/...` at the established baseline, no new failures; 10x stress run of the new/tightened tests clean.
- `[x]` **DOD-DOC — Docs.** This doc moved to `done/` with a Validation section; `change-audit/CA-235-flow-step-sync-settlement-and-reviewer-findings-fallback.md` written.

## 12. Code Change Plan (per DoD item)

### DOD-1 / DOD-2 — synchronous step-status writes

- `interactive_service.go` `applyFlowControl` "continue" case: moved the `looping` branch's downstream-`PENDING`/entry-`RUNNING` reset and the `awaiting_user` branch's `setFlowStepAwaitingUser` call out of `go func(){...}()`/`go s...(...)` to run synchronously, ahead of the (now-relocated-after) `s.emitAgentGraph(parentRunID, snap)` call. The coder-reentry (`maybeReinvokeCoderForContinue`) call stays a goroutine (unrelated network/turn-start work).
- `interactive_service.go` `applyFlowControl` "escalate" case: `s.setFlowStepAwaitingUser(...)` moved before `s.emitAgentGraph(...)`, no longer a goroutine.
- `interactive_service.go` `resumeFlowWithFeedback`: the hub-node `StepStatusRunning` transition moved before `s.emitAgentGraph(...)`, no longer a goroutine.
- `interactive_service.go` `emitLocked`'s `EventTurnCompleted` cohort-join branch: the reviewer-`DONE`/hub-`RUNNING` writes pulled out of the `go func(){...}()` that also calls `maybeAutoReinvokeHubWithNote` — they now run synchronously (under the already-held `s.mu`, safe because `workflowStore.ApplyStepTransition` has its own lock) immediately before that goroutine is spawned with only the reinvoke call left inside it.
- Tests: `TestApplyFlowControlEscalateSettlesHubToWaitingUser`, `TestApplyFlowControlCapReachedSettlesHubToWaitingUser` (both tightened to immediate assertions), `TestApplyFlowControlLoopingResetsStepsSynchronously` (new), `TestResumeFlowWithFeedbackAutoExtendsOnlyForCap`'s cap sub-test (extended).

### DOD-3 / DOD-4 — reviewer-findings fallback content

- `interactive_service.go`: new `interactiveRun.lastCohortNote string` field; populated at both cohort-join sites (`EventTurnCompleted` and `EventTurnFailed` branches of `emitLocked`) right after `buildCohortNote` runs, alongside the existing `appendPendingAgentContextLocked` call.
- `interactive_service.go`: new `lastCohortNoteFor(runID string) string` (lock-guarded getter) and `summarizeCohortNoteForUser(note string) string` (strips the engine-internal header/instructions, keeps only per-reviewer lines, truncated via the existing `truncateDisplayField` helper to 800 chars) — both placed next to `buildCohortNote`.
- `interactive_service.go` — the CA-226 fallback (hub-turn-completion handler): builds `Summary` from `summarizeCohortNoteForUser(s.lastCohortNoteFor(rs.id))` when non-empty, falling back to the original diagnostic sentence otherwise; removed the subsequent `s.setFlowStepStatus(ctx, rs.id, hubID, StepStatusFailed)` call (applyFlowControl's escalate case already settles it correctly).
- Tests: `TestE2EReviewLoopSynthesisFallbackEscalates` (updated assertions), `TestSummarizeCohortNoteForUserStripsEngineInstructions`, `TestSummarizeCohortNoteForUserEmptyInputReturnsEmpty` (new, `agent_orchestrator_test.go`).
