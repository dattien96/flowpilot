# BUG-234: Review-Loop Back-Edge Lifecycle — Runaway Auto-Advance After Block, Cohort Node Settlement, And Loop-Back Timeline Legibility

## Metadata

- Document ID: `BUG-234`
- Title: `Review-Loop Back-Edge Lifecycle — Runaway Auto-Advance After Block, Cohort Node Settlement, And Loop-Back Timeline Legibility`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [BUG-231: Escalate/Awaiting-User Flow State Is Not Actionable](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md), [BUG-233: Flow Timeline Staleness And Blocked-Card Diagnostic Content](../done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md), [CA-226: Hub Synthesis Fallback Escalation](../../../change-audit/CA-226-hub-synthesis-fallback-escalation.md), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, review-loop, flow-control, cohort, step-timeline, loop-back, desktop`

## AI Quick View

### Summary

Live testing of a multi-round review loop (reviewer requested changes → looped back to the coder → reviewers re-ran) surfaced four issues, all rooted in the loop-back / cohort lifecycle:

1. **Cohort node settlement (Ảnh 1, 3):** when one reviewer of a cohort finishes but the other is still running, the step timeline shows BOTH reviewer nodes as `RUNNING`. A reviewer's own timeline node is only settled to `DONE` at the cohort barrier (all members joined), never on its own completion.
2. **No loop-back indicator (Ảnh 2):** after a loop-back, the step timeline shows the same 4 node rows with no sign that a new round started — the row set is keyed per node id, not per round, and the loop `Round` counter is never surfaced.
3. **Coder reused vs reviewers re-spawned (Ảnh 4):** each loop-back re-prompts the SAME coder run (one run, many turns) but spawns NEW reviewer runs, so the panel shows e.g. "4 reviewers, 1 coder". The user asked WHY — see `D-3`.
4. **Runaway auto-advance after block → flow never completes (Ảnh 5, the serious one):** in a looped run where the synthesizer does not cleanly approve/continue (e.g. a prose-only turn, or a genuine escalate), the CA-226 fallback correctly escalates the loop to `blocked` and settles the hub node to `WAITING_USER_APPROVAL` — but the flow keeps auto-advancing: reviewers re-spawn, the cohort re-joins, and the hub node is flipped back to `RUNNING` over and over. The synthesis step spins forever and the run never settles.

### Current Ask

- Investigate (done, this doc) and fix #1, #2, #4-serious. #3 is a "why" question answered below and largely mitigated by the #4 fix (no more runaway reviewers); a behavior change (make the coder also spawn-new, or make `Lifecycle` actually drive reuse) is deferred unless requested.

### Key Decisions

- `D-1` **(#4/Ảnh 5 root cause — the fix that matters most)** The auto-advance paths do NOT gate on loop status. `advanceOrNotifyHub` → `tryAdvanceFlowFromNode` (spawns the next nodes on a forward edge) and the cohort-join reviewer-DONE / hub-RUNNING write both run unconditionally on any child completion. The only loop-status helper on the coder-reentry path, `loopAllowsNextTurnLocked` (interactive_service.go:1065), checks only `paused`/`stopped` — NOT `blocked`/`done`. `maybeAutoReinvokeHub` IS gated (its `switch st.Status { case "paused","stopped","blocked","done": return }`), which is why the loop STATE is correct while the STEPS/reviewers run away. Fix: gate auto-advance (and the cohort-join re-spawn/hub-RUNNING write) on an active loop status, mirroring `maybeAutoReinvokeHub`.
- `D-2` **(#1)** A cohort member's own timeline node must settle to `DONE` (or `FAILED`) on its OWN completion, independent of the barrier, using its label (= node id). The hub-RUNNING transition still waits for the barrier. This is additive to the barrier's bulk reviewer-DONE write, which becomes a harmless idempotent no-op once each member self-settled.
- `D-3` **(#4 "why")** The coder/reviewer difference is NOT configured per node. Both `coder` and `reviewer_*` nodes declare `lifecycle: reinvoke` in the pack (`review-loop.yaml`), but `Lifecycle` is a DEAD attribute — only defaulted/normalized (`agent_orchestrator.go:539`), never read in any spawn-vs-reuse decision. The behavior is driven purely by edge `Kind`: a `back`/`continue` edge routes to `maybeReinvokeCoderForContinue` (re-prompts the existing run), a `forward`/`done` edge routes to `tryAdvanceFlowFromNode` (spawns new runs). So the coder (reached by a back edge) is reused and reviewers (reached by forward edges) are re-spawned. This is an emergent artifact, not a designed per-node choice. Decision: keep the current reuse/spawn behavior for now (changing it is a larger design decision); the runaway-reviewer inflation is fixed by `D-1`. Documented so the dead `Lifecycle` attribute is not mistaken for a control.
- `D-4` **(#2)** Surface the loop `Round` on the step timeline so a loop-back is legible. The loop state already carries `Round`; expose it (e.g. a "Round N" badge on the timeline / a per-node round marker) rather than inventing per-round step rows (which would be a much larger model change). Exact desktop presentation is a UI detail; the backend already emits `Round` in the agent-graph snapshot.

### Open Questions

- `Q-1` Should the coder eventually be spawned fresh per round (consistency with reviewers) or keep the reuse-the-thread model? Deferred — see `D-3`. Not needed to resolve the hang.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go:958-968` — `advanceOrNotifyHub` → `tryAdvanceFlowFromNode` with no loop-status guard (the runaway root).
- `apps/local-runner/internal/runner/interactive_service.go:1063-1066` — `loopAllowsNextTurnLocked` checks only `paused`/`stopped`.
- `apps/local-runner/internal/runner/interactive_service.go` cohort-join branch (`emitLocked`, `EventTurnCompleted`, `cohortComplete`) — reviewer-DONE + hub-RUNNING writes run unconditionally; individual member completion (the non-`cohortComplete` path) never settles the member's own node.
- `apps/local-runner/internal/runner/flow_executor.go:349-431` — `tryAdvanceFlowFromNode` spawns forward-edge targets; `flow_executor.go` `maybeReinvokeCoderForContinue` (interactive_service.go:982) reuses the existing run on a back edge.
- `apps/local-runner/internal/runner/agent_orchestrator.go:539-540` — `Lifecycle` defaulted, never read for spawn-vs-reuse.
- `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml:30,45,54` — both delegate nodes declare `lifecycle: reinvoke`; synthesis is `hub.inline`.
- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx` — one row per node; `retryCount` badge exists but the flow-engine loop-back reset never bumps it, and `Round` is not shown.

## 1. Issue Summary

A multi-round review loop exposes four loop-back/cohort lifecycle defects: (1) cohort members don't settle their own timeline node until the barrier; (2) no loop-back/round indicator on the timeline; (3) coder is reused while reviewers are re-spawned (a "why", from a dead config attribute + edge-kind routing); and (4, serious) auto-advance is not gated on loop status, so after the loop is legitimately `blocked`/escalated the flow keeps re-spawning reviewers and re-running the hub node, never settling — the run appears to hang on the synthesis step.

## 2. Parent Links

Surfaced during live validation of [BUG-231](../done/BUG-231-Escalate-Awaiting-User-State-Not-Actionable-Flow-Hangs.md) / [BUG-233](../done/BUG-233-Flow-Timeline-Staleness-And-Blocked-Card-Diagnostic-Content.md). The escalate/awaiting-user settlement those bugs added is correct; this bug is that the flow does not STOP advancing once it reaches that settled state.

## 3. Environment and Reproduction

- Built-in "Review Loop", flow-engine-driven, 2 reviewers, multi-round (changes requested → loop back → reviewers re-run).
- #4 reproduced deterministically by a faithful E2E test: drive the real flow via `startResolvedFlow` with `flowEngineDriven=true`, have the round-1 synthesis request changes and the round-2 synthesis COMPLETE WITHOUT calling `submit_review_outcome` (prose-only — the realistic Haiku failure). Result: loop settles to `blocked`/`escalate` and hub node to `WAITING_USER_APPROVAL`, then reviewers re-spawn and the hub node flips back to `RUNNING` repeatedly.
- The clean multi-round path (changes → approved → done) is CORRECT and does NOT hang (proven by `TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes`, 50× green) — the bug only manifests once the loop enters `blocked`/escalate.

## 4. Expected vs Actual

- #1 — Expected: a finished reviewer's node reads DONE while its sibling still runs. Actual: both read RUNNING until the barrier.
- #2 — Expected: a loop-back is visibly a new round. Actual: same rows, no round cue.
- #4 — Expected: once the loop is `blocked`/awaiting-user, the flow STOPS advancing and the synthesis step stays `WAITING_USER_APPROVAL`. Actual: reviewers re-spawn, cohort re-joins, hub node flips back to `RUNNING`, forever.

## 5. Impact

- #4 is a functional hang: a looped run that ends in escalate/block never settles and keeps consuming provider budget (runaway reviewer spawns). It also inflates the reviewer count (#4/Ảnh 4). This is the most severe.
- #1/#2 are legibility bugs that make the timeline misleading during a loop.

## 6. Root Cause

- **#4:** auto-advance (`advanceOrNotifyHub`/`tryAdvanceFlowFromNode`) and the cohort-join reviewer-DONE/hub-RUNNING write have no active-loop-status guard; `loopAllowsNextTurnLocked` omits `blocked`/`done`. `maybeAutoReinvokeHub` is gated, so the loop state settles but the step/spawn machinery does not stop.
- **#1:** individual cohort-member completion only buffers the result (`appendCohortResult`); the member's own node is settled only in the `cohortComplete` bulk write.
- **#2:** the step timeline is keyed one-row-per-node-id and never surfaces `Round`; the flow-engine loop-back reset uses `setFlowStepStatus(...PENDING)` which does not bump `retryCount`.
- **#4/Ảnh 4 "why":** `Lifecycle` is a dead attribute; reuse-vs-spawn is edge-`Kind`-driven (back→reuse, forward→spawn).

## 7. Fix Strategy

- #4: add an "is the loop still advancing?" guard (active = not `paused`/`stopped`/`blocked`/`done`) and apply it at the top of `tryAdvanceFlowFromNode` and to the cohort-join reviewer-DONE/hub-RUNNING + reinvoke block, so a completion arriving after the loop is blocked/done does not spawn or re-run anything.
- #1: in the individual cohort-member completion path (both `EventTurnCompleted` and `EventTurnFailed`), settle that member's own node (`rs.label`) to `DONE`/`FAILED` for flow-engine-driven runs, before/independent of the barrier.
- #2: surface `Round` on the step timeline (desktop) — a "Round N" badge driven by the loop state already in the agent-graph snapshot. Minimal backend change (ensure `Round` is available to the timeline).
- #4/Ảnh 4: document the dead `Lifecycle` attribute; no behavior change this round.

## 8. Validation

- `go build ./...` / `go vet ./internal/runner/...` — clean. Desktop `npm run typecheck` / `npm run build` — clean.
- `go test ./internal/runner/...` — 13 pre-existing environment-specific failures only (Windows paths, missing Codex CLI, Google-Drive, skills-merge — identical to the BUG-233 baseline); no flow/cohort/review-loop failures introduced.
- New/updated Go tests, all green (flow/cohort subset re-run 4×, 144/144):
  - `TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes` — clean multi-round changes→approved→done (proves the happy path was never the bug); 50× stable.
  - `TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes` — forces the deferred-reinvoke window; multi-round still completes.
  - `TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing` (the #4 regression) — a round-2 prose-only synthesis escalates; asserts the loop STAYS blocked, the hub node STAYS `WAITING_USER_APPROVAL` (never flaps back to RUNNING), and reviewer turns SETTLE (no unbounded runaway); 12× stable.
  - `TestCohortMemberSettlesOwnNodeOnCompletion` (the #1 regression) — one cohort member completing settles its own node to DONE while the sibling is still RUNNING; 5× stable.
- Method note: the #4 root cause was found by faithfully reproducing the realistic failure (round-2 synthesis completes prose-only with `flowEngineDriven=true`) after the happy-path multi-round tests passed 50× — which disproved the earlier line-854 cap-gate hypothesis. A temporary stack-trace probe pinned the runaway to `advanceOrNotifyHub → tryAdvanceFlowFromNode` spawning into an already-blocked loop (a check-then-spawn gap), fixed by gating at entry AND per-spawn.

## 9. Regression Guard

- #4: an E2E test that drives a round-2 prose-only synthesis with `flowEngineDriven=true` and asserts, after a settle delay, that the loop is `blocked`, the hub node is `WAITING_USER_APPROVAL` (not `RUNNING`), and no further reviewer runs were spawned after the block.
- #1: a test asserting a single cohort member's completion settles its own node to `DONE` while the sibling is still `RUNNING`.

## 10. Follow-Up Document Updates

- SD-19: note the auto-advance active-status guard as part of the flow-control contract.

## 11. Definition of Done

- `[x]` **DOD-1 — #4 auto-advance gate.** `loopIsAdvancing` helper added; `loopAllowsNextTurnLocked` now also excludes `blocked`/`done`; `tryAdvanceFlowFromNode` gates at entry AND re-checks per-spawn; the cohort-join hub-RUNNING write is gated on `loopIsAdvancing`.
- `[x]` **DOD-2 — #4 regression test.** `TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing` (12× stable) — settled `blocked` + hub `WAITING_USER_APPROVAL` that stays settled, reviewer turns bounded/settled.
- `[x]` **DOD-3 — #1 per-member node settlement.** Cohort member settles its own node DONE (completed) / FAILED (failed) on its own completion; `TestCohortMemberSettlesOwnNodeOnCompletion` (5× stable).
- `[x]` **DOD-4 — #2 round indicator.** `FlowTimelineSidebar` shows a "Round N/cap" badge from the loop state, highlighted once looped back.
- `[x]` **DOD-5 — #4/Ảnh 4 documented.** The dead `Lifecycle` attribute + edge-kind routing explained (`D-3`); no behavior change.
- `[x]` **DOD-6 — Full regression green.** `go build`/`vet`/`test` at the established baseline (only the pre-existing environment failures); desktop `typecheck`/`build` clean.
- `[x]` **DOD-DOC — Docs.** This doc moved to `done/`; `change-audit/CA-236-*` written; SD-19 updated.

## 12. Code Change Plan (per DoD item)

### DOD-1 — auto-advance active-status gate
- Add a helper (e.g. `loopIsAdvancingLocked(parentRunID) bool` returning `status ∉ {paused, stopped, blocked, done}`), or reuse the same status set `maybeAutoReinvokeHub` uses.
- Guard `tryAdvanceFlowFromNode` (flow_executor.go) at entry: if the loop is not advancing, return false (do not spawn forward targets).
- Guard the cohort-join block in `emitLocked` (`cohortComplete` branch): if the loop is not advancing, skip the reviewer-DONE/hub-RUNNING writes and the `maybeAutoReinvokeHubWithNote` call (the reinvoke is already gated, but skipping the hub-RUNNING write prevents the flap).
- Optionally extend `loopAllowsNextTurnLocked` callers (coder re-entry) — but that path is only reached from `applyFlowControl("continue")`, which cannot fire while blocked; lower priority.

### DOD-3 — per-member cohort node settlement
- In `emitLocked`'s `EventTurnCompleted` cohort branch (before/independent of `cohortComplete`): if flow-engine-driven, `setFlowStepStatus(parentRunID, rs.label, StepStatusDone)`.
- In the `EventTurnFailed` cohort branch: same with `StepStatusFailed`.

### DOD-4 — round indicator
- Backend: ensure the loop `Round` reaches the desktop step-timeline (already in the agent-graph snapshot loop state; confirm the timeline has access).
- Desktop (`FlowStepTimeline.tsx` / its container): render a "Round N" badge when `Round > 1` (or always), sourced from the loop state.

### DOD-2 / regression tests
- Go: add `TestE2EReviewLoopBlockedStopsAdvancing` (prose-only round-2 → settled and stays settled, no post-block spawn); keep the two multi-round tests already added.
- Go: add a cohort per-member settlement test.
