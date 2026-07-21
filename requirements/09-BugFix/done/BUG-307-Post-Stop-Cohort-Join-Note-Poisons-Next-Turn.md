# BUG-307 — Post-Stop cohort join note poisons the next turn (hub watchdog stalls)

## Metadata

- Document ID: `BUG-307`
- Title: A cohort that finishes joining after the parent loop already sealed (stopped/done) leaves its synthesis note queued to leak into the next turn
- Phase: `bugfix`
- Status: `done`
- Owner: local-runner
- Reviewers: n/a
- Created: 2026-07-21
- Last Updated: 2026-07-21
- Parent Documents: BUG-305 (companion — live-hang half of the same A8/A10 verification pass), BUG-302 (post-"done" follow-ups), BUG-122 (agent-context note prefix), BUG-233/BUG-275 (lastCohortNote / duplicate-note guards), CP-51 A8/A11
- Child Documents: none
- Related Documents: BUG-306 (sibling — a different stale-context leak into a restart-reconstructed transcript)
- Replaces: none
- Tags: agent-flow-engine, cohort, hub-stall, resume, regression

## AI Quick View

### Summary

- Found live on run-19500 (Review Loop, `fix bug 1+1 != 2`) while verifying CP-51 A8: Stop cancelled 2 in-flight reviewers, both joined the cohort as the Stop request's own cancellation cleanup — correctly, per A8 (see CP-51 A8 evidence: stop-generation fence held, no revival). The server was later restarted and a plain follow-up ("lan truoc fix gi vay") on the same run hung for 2 minutes before the hub-stalled watchdog surfaced a "no progress" card.
- Root cause: 6 call sites build a cohort-join synthesis note (`buildCohortNote`, "...then call the flow's control tool with the consolidated result") and queue it to `pendingAgentContext` **unconditionally** — including when the cohort completes its join AS PART OF the Stop request itself, after `agentOrchestrator.stop()` has already sealed the loop. `maybeAutoReinvokeHubWithNote`'s own status switch blocks the reinvoke that would drain the note, but does not undo the append. The stranded note then gets silently prepended (BUG-122's `composeAgentContextBlock`) to the next turn on the run, whatever that turn is — including an ordinary chat follow-up typed after a restart. The model receives "Synthesize... call the flow's control tool" with no live flow left to satisfy it, and the hub-stall watchdog (BUG-289 F-0) times out waiting for progress that can never come.

### Current Ask

- Stop a poisoned synthesis note from ever reaching a later turn's prompt once the loop that would have consumed it has permanently sealed, without breaking the legitimate "blocked/paused → Continue" resume path that genuinely needs the queued note, and without regressing any of the many prior cohort-join/hub-reinvoke fixes in this area (BUG-233, BUG-275, BUG-289, BUG-302, run1618, run333, run9437).

### Key Decisions

- `V-1` New predicate `loopSealedForReinvoke(parentRunID)`: true only for loop status `"stopped"` or `"done"` — the two states that will never schedule another hub reinvoke. `"blocked"`/`"paused"` are deliberately excluded (unlike the existing `loopIsAdvancing`, which also excludes those two for a different, UI-flip purpose) because a later Continue/Resume legitimately drains the queued note for those.
- `V-2` Guard is applied at the **append** call sites, not by clearing `pendingAgentContext` wholesale on Stop — other queued notes (e.g. "Sub-agent X completed: ...", "Flow completed. Summary: ...") carry no imperative instruction and remain valid context for a plain post-stop/post-done follow-up (BUG-302's whole point). Only the cohort-synthesis note's "call the flow's control tool" instruction is toxic once no flow is left to run it.
- `V-3` `parent.lastCohortNote` is set **unconditionally**, regardless of the guard — it is diagnostic-only (BUG-233's CA-226 GateReason fallback), never sent to a provider, so a stopped run's timeline can still show what the cancelled reviewers found.
- `V-4` All 6 live, in-session call sites that build+queue a cohort-join note got the same guard (interactive_service.go: the Stop-handler's own cohort-cancel join, the pre-flight child-failure join, the `EventTurnCompleted` cohort join, the `EventTurnFailed` cohort join, and `maybeAutoReinvokeHubWithNote`'s own busy-defer re-append; cohort_stall.go: the stall-timeout member-skip join). The boot/crash-reconstruction path (`interactive_resume.go`'s `joinRecoveredCohort`) was deliberately left alone — at that point in the resume flow the in-memory `agentOrchestrator.loop` status has not yet been seeded from durable state (a separate, deeper question), so guarding it the same way risked silently dropping a legitimate note for a genuinely-recovering run; flagged as a follow-up, not fixed here.

### Constraints

- additive-tests-only: only `bug307_post_stop_cohort_join_poisons_pendingcontext_test.go` was added; no existing test file was modified.
- cross-provider-parity: the fix lives entirely in shared orchestration code (no provider adapter branches on `ProviderKey` anywhere in the touched functions); the Grok-tagged variant test proves the mechanism is provider-agnostic.
- Must not regress `TestStopAgentLoopCancelsCohortMemberAndJoins` (the existing BUG-233/Task-241 sibling test covering the same Stop-cancel-join path) or any of the hub-stall/resume-continue battery.

### Open Questions

- None for this fix's scope. Flagged-not-fixed: whether `interactive_resume.go`'s `joinRecoveredCohort` (crash-mid-cohort recovery) has the same latent risk once `agentOrchestrator.loop` status is reliably seeded from durable state at boot — needs its own investigation + repro, not assumed here.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` (`loopSealedForReinvoke`, `loopIsAdvancing`, `stopAgentLoop`'s cohort-cancel join, `handleChildStartTurnFailure`'s pre-flight join, `settleFlowChildTurnCompletedLocked`'s completed/failed cohort joins, `maybeAutoReinvokeHubWithNote`)
- `apps/local-runner/internal/runner/cohort_stall.go` (`checkAndBlockStalledMembers`'s stall-skip join)
- `apps/local-runner/internal/runner/bug307_post_stop_cohort_join_poisons_pendingcontext_test.go`

## 1. Issue Summary

A cohort (e.g. 2 review-loop reviewers) can complete its join barrier at the exact moment the parent loop is being sealed by Stop, or after it already finished ("done"). The join handler always queues a "synthesize and call the flow's control tool" note into `pendingAgentContext` for the next turn to consume — but if the loop is sealed, no reinvoke will ever consume it, and it instead poisons whatever ordinary turn comes next on that run.

## 2. Parent Links

- impacted coding plan: CP-51 A8 (found during verification), CP-51 A11 (new manual regression scenario)
- impacted tech design: BUG-289 F-0 hub-stall watchdog; BUG-302 post-done follow-up admission; BUG-122 agent-context note wrapper
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: any; requires a flow-engine cohort (e.g. Review Loop reviewers) still in-flight when Stop is pressed (or a late/failed join after "done"), followed by any later turn on the same run (a hub reinvoke attempt, or — as observed live — an ordinary chat follow-up after a restart).
- reproduction (live): run-19500 — Stop while 2 reviewers in-flight → both cancelled/joined → restart → follow-up "lan truoc fix gi vay" hung 2m0s → hub-stalled "no progress" card.
- reproduction (test): `go test ./internal/runner -run TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContext` — fails without the fix with `pendingAgentContext still carries the stale synthesis note after Stop`.

## 4. Expected vs Actual

- expected: once Stop cancels the remaining cohort member and the join completes, no further reinvoke-oriented note should be queued for a future turn to accidentally inherit; `lastCohortNote` still records the outcome for diagnostics.
- actual: the note was queued regardless, and the next unrelated turn's prompt silently inherited a "call the flow's control tool" instruction for a flow that no longer exists to run.

## 5. Impact

- users affected: anyone who stops a Review-Loop-style flow while a cohort is still in-flight (or hits a late/failed join right after the flow finishes) and later sends any other turn on that run — across Codex/Claude/Grok.
- workflows affected: post-Stop / post-done follow-up chat, hub-reinvoke scheduling.
- severity: medium (no data loss; the affected turn hangs for the full hub-stall timeout — 2 minutes by default — before surfacing an operator card; recoverable via the existing Continue/Stop card, but confusing and slow).

## 6. Root Cause

- confirmed cause: 6 call sites build a cohort-join note via `buildCohortNote` and append it to `pendingAgentContext` without checking whether the loop that would ever schedule a note-consuming reinvoke has already sealed. `maybeAutoReinvokeHubWithNote`'s status switch (`"paused", "stopped", "blocked", "done"` → no-op) prevents the *reinvoke*, but every one of the 6 append call sites runs *before or independently of* that check, so the note itself is never retracted.
- evidence: `runner.log` for run-19500 — cohort members `turn_failed` at the same timestamp Stop was requested; `[dispatch] terminal commit failed ... dispatch record revision is stale` confirms the loop had already sealed by the time the join settled; the restored follow-up turn's outgoing prompt (captured in the same log) is the wrapped join note verbatim ("Flow round 0 — 2 results joined... Synthesize... call the flow's control tool"), sent alongside the user's real "lan truoc fix gi vay" message.

## 7. Fix Strategy

- `F-1` Add `loopSealedForReinvoke(parentRunID)`, true only for `"stopped"`/`"done"`.
- `F-2` Guard the `appendPendingAgentContext(Locked)` call at each of the 6 cohort-join-note sites with `!loopSealedForReinvoke(...)`; leave `lastCohortNote` assignment unconditional at every site (diagnostic-only, never sent to a provider).
- `F-3` Leave `isSystemPrompt`/`composeAgentContextBlock`/the boot-recovery `joinRecoveredCohort` path untouched — this fix is confined to whether a note is queued at all, not to how a queued note later gets classified or rendered.

## 8. Validation

- `V-1` **cross-provider-parity:** none of the 6 touched call sites branch on `ProviderKey`; `TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContextGrok` proves the same scenario with Grok-tagged children.
- `V-2` **additive-tests-only:** only the new BUG-307 test file added; no existing test modified.
- `V-3` **git-stash regression discipline:** with the fix stashed, both `TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContext` and its Grok variant fail with the stale note still present in `pendingAgentContext`; with the fix restored, both pass alongside `TestLoopSealedForReinvoke` (unit-pins the exact sealed/non-sealed status set).
- `V-4` **prior-fix invariants preserved:** `TestStopAgentLoopCancelsCohortMemberAndJoins` (the existing Task-241/BUG-233 sibling covering the same Stop-cancel-join path) and the wider hub-stall/resume-continue battery (BUG-289, run1618, run333, run9437, `TestResumeFlowWithFeedback*`, `TestCohort*`, `TestLiveStop*`) all pass unchanged — 56 tests green.
- `V-5` **full-suite regression:** `go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1` — 18 failures, all pre-existing environment-only (Windows paths, external `codex`/`git` CLI availability) — no new failures.
- `go build ./...` clean.

## 9. Regression Guard

- tests: `TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContext` (Codex), `…Grok` (cross-provider), `TestLoopSealedForReinvoke` (unit); the existing `TestStopAgentLoopCancelsCohortMemberAndJoins` continues to guard the untouched `lastCohortNote`/join-completion behavior.
- alerts: n/a
- audit checks: none.

## 10. Follow-Up Document Updates

- upstream docs that must change: CP-51 A8 note updated to reference this fix; CP-51 A11 added as a new manual verification scenario for the exact "post-stop, restart, then follow-up" hang.
- notes left unchanged on purpose: `interactive_resume.go`'s `joinRecoveredCohort` (crash/boot-recovery cohort join) was not given the same guard — flagged as an open follow-up rather than fixed speculatively, since the in-memory loop-status signal it would need to check is not reliably seeded at that point in the resume lifecycle.
