# CA-236: Gate Auto-Advance On Loop Status; Per-Member Cohort Node Settlement; Loop-Back Round Indicator

## Summary

BUG-234 tracked four issues from live multi-round review-loop testing. The serious one (#4): after a looped run legitimately blocked (an escalate awaiting the user), the flow kept auto-advancing — reviewers re-spawned and the hub node flipped back to `RUNNING` repeatedly, so the synthesis step spun forever and the run never settled (the "can't complete the loop" hang). The others: a finished cohort member's node stayed `RUNNING` until the barrier (#1), no loop-back/round indicator on the timeline (#2), and a "why are there 4 reviewers but 1 coder" question (#4/Ảnh 4). Fixed the two functional bugs (#1, #4) and the legibility gap (#2); answered #4/Ảnh 4 (a dead config attribute — no behavior change).

Root cause of #4 was pinned by reproduction, not inspection: the clean multi-round path (changes→approved→done) passes 50×, which disproved an earlier cap-gate hypothesis. Faithfully reproducing the realistic Haiku failure (a round-2 synthesis that completes prose-only, with `flowEngineDriven=true`) reproduced the hang; a temporary stack-trace probe showed `advanceOrNotifyHub → tryAdvanceFlowFromNode` spawning reviewers into an already-`blocked` loop, because the auto-advance paths had no loop-status guard (only the hub reinvoke did).

## What Changed

### Runner (`apps/local-runner`)

- `interactive_service.go` — new `loopIsAdvancing(parentRunID)` (active = status ∉ {`paused`,`stopped`,`blocked`,`done`}, the same set `maybeAutoReinvokeHub` gates on). `loopAllowsNextTurnLocked` now delegates to it, so it ALSO excludes `blocked`/`done` (previously only `paused`/`stopped`) — closing the dependent-agent release/resume and coder-reentry runaway paths.
- `flow_executor.go` `tryAdvanceFlowFromNode` — gates on `loopIsAdvancing` at entry AND re-checks before each `spawnChildRun` in the target loop, closing the check-then-spawn race where a concurrent hub synthesis turn escalates mid-advance.
- `interactive_service.go` cohort-join (`emitLocked`, `EventTurnCompleted`) — the hub-`RUNNING` transition is now gated on `loopIsAdvancing` so a late/stray cohort join can't flip the settled hub node back to `RUNNING`.
- `interactive_service.go` cohort-member completion (both `EventTurnCompleted` and `EventTurnFailed`) — a flow-driven member now settles its OWN node (`rs.label`) to `DONE`/`FAILED` on its own completion, independent of the barrier (#1).

### Desktop (`apps/desktop-flowpilot`)

- `components/FlowTimelineSidebar.tsx` — new "Round N/cap" badge sourced from the loop state (`agentGraphSnapshot.loopState.round`, 0-based → displayed +1), highlighted once the flow has looped back (#2).
- `styles.css` — `.flow-sidebar-round` / `.flow-sidebar-round-active`.

### Docs

- `requirements/09-BugFix/done/BUG-234-...md` — full investigation, decisions (incl. `D-3`: the `Lifecycle` attribute is dead; reuse-vs-spawn is edge-`Kind`-driven), DoD, code change plan, validation; moved to `done/`.
- `requirements/06-System-Tech-Design/SD-19-Agent-Flow-Engine.md` §8 `F-3` — added the BUG-234 auto-advance gate and cohort-node-settlement contract notes.

### Tests (`apps/local-runner`)

- `TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes` (new) — clean multi-round changes→approved→done; the coverage gap that let this regress.
- `TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes` (new) — forces the deferred-reinvoke window; still completes.
- `TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing` (new, the #4 regression) — a prose-only round-2 escalate: loop stays blocked, hub node stays `WAITING_USER_APPROVAL` (no flap), reviewer turns settle (no runaway).
- `TestCohortMemberSettlesOwnNodeOnCompletion` (new, the #1 regression) — one member completing settles its own node `DONE` while the sibling is still `RUNNING`.

## Verification

- `go build ./...` / `go vet ./internal/runner/...` — clean. Desktop `npm run typecheck` / `npm run build` — clean.
- `go test ./internal/runner/...` — 13 pre-existing environment-specific failures only (Windows paths, missing Codex CLI, Google-Drive, skills-merge — identical to the BUG-233 baseline); no flow/cohort/review-loop failures introduced.
- New/flow-related tests re-run 4× (144/144); the two timing-sensitive new tests re-run individually 12× / 5× — 0 failures.
- Not executed: a live click-through in the running desktop app (same sandbox limitation as CA-233/234/235 — no reachable local-runner backend). The round-indicator is a presentational addition over the existing loop-state plumbing; the functional fixes are covered by the Go tests above.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-234
change_type: bugfix
summary: Stop auto-advance (reviewer re-spawn, hub-RUNNING flip, dependent release) once a review loop has blocked/settled so a looped run no longer spins forever; settle each cohort member's own step node on its own completion; add a loop-back round indicator
# --->8---
