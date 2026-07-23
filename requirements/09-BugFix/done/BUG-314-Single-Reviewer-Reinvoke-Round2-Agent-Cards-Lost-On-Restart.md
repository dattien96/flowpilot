# BUG-314: Single-Reviewer Reinvoke Round-2 Agent Cards Lost On Restart

## Metadata

- Document ID: `BUG-314`
- Title: `A Review Loop flow with only one reviewer (no cohort) loses every round after the first when its parent chat is reopened after a server restart`
- Phase: `bugfix`
- Status: `done`
- Owner: `agent-flow-engine`
- Reviewers: `TBD`
- Created: `2026-07-23`
- Last Updated: `2026-07-23`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine And Recovery Reconciliation](../../07-Coding-Plan/inprogress/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md)
- Child Documents: `none`
- Related Documents: [BUG-294: Resumed Agent Card Shows "— completed" Suffix On A Cancelled Child](../done/BUG-294-Resumed-Agent-Card-Shows-Completed-Suffix-On-Cancelled-Child.md) (same `resumedParentAgentAnnotations` reconstruction path), [BUG-313: Restored Chat Timeline Broken — Sync Never Carried The Turn Log](../done/BUG-313-Restored-Chat-Timeline-Broken-Sync-Never-Carried-Turn-Log.md) (found while cross-checking the same "reconstruct timeline after restart" contract for Drive restore; this bug is the same-machine-restart half, not the Drive-sync half)
- Replaces: `none`
- Tags: `agent-flow-engine, chat-ui, resume, restart, review-loop, cross-provider, regression`

## AI Quick View

### Summary

- Operator ran the same Review Loop flow shape on Codex, Claude, and Grok (runs `run-46797`/`run-53157`/`run-45881`). Codex and Grok restored correctly after a server restart. The Claude run had genuinely looped to a second round (visible live: coder committed a fix, reviewer requested changes, coder re-ran and committed again) — but reopening the chat after restart showed only round 1's cards; round 2 (both the coder's and the reviewer's second activation) had vanished.
- Root cause: the restored agent-card count for a reused ("reinvoke"-lifecycle) flow node came from a turn-log prompt-count heuristic (`resumeChildActivationCount`), capped by a "peer start-time wave" heuristic (`peerStartWaveTimes` inside `resumedParentAgentAnnotations`) that infers a second Review Loop round from *other* children's start times. The Claude flow used exactly one reviewer (no cohort), so there was no other child to form a wave from — the cap collapsed every reused node to a single activation regardless of how many times it actually ran. Codex/Grok were unaffected only because their flows happened to run 2+ reviewers per round, which supplied the wave signal this heuristic depends on.
- Fixed by reading the durable per-node step-transition sidecar (Task-239, `<runID>-step-transitions.ndjson`) when it unambiguously belongs to one child — each `RUNNING` transition there is a real activation, verified live to reproduce the fix and non-regression across all three providers.

### Current Ask

- Fixed. `resumedParentAgentAnnotations` now computes a reused node's activation count and timing from its step-transition `RUNNING`→terminal pairs when exactly one child run claims that node's label (`labelCounts`); falls back to the pre-existing turn-log-heuristic + peer-wave-cap path otherwise (no step-transition sidecar, or the label is legitimately shared by several distinct child runs — Codex's `reviewer_correctness`/`reviewer_security`, which get a fresh run id every round and were already correct).

### Key Decisions

- `V-1` Prefer the step-transition log's `RUNNING` transitions as ground truth over turn-log prompt counting: a mid-round gate reprompt ("missing change-audit note") lands as an extra turn-log prompt line without being a real new activation, which is exactly what over-counted the Claude coder to 3 heuristic activations for only 2 real ones. `RUNNING` transitions don't have this failure mode — the flow engine only re-enters `RUNNING` when it genuinely reinvokes the node.
- `V-2` Gate the new path on `labelCounts[child.agentName] == 1` (exactly one child run claims this label in the whole hub). A reinvoke-lifecycle node keeps one run id across every round, so it is always the sole claimant of its label — safe to expand from the log. A spawn-lifecycle node gets a fresh run id each round, so its label is claimed by several distinct children; expanding any single one of them from the (shared-label) log would incorrectly attribute every round's pairs to that one child. This guard is what keeps Codex's already-correct multi-round reviewer cards at exactly one each.
- `V-3` No behavior change when the step-transition sidecar is absent or has no entries for a label (`LoadStepTransitions` returns `nil, nil` for a missing file) — falls straight through to the exact pre-existing heuristic/cap code path. This is what keeps every prior resume-order test (`run5695`, `run9034`, `run1264`, `run24377`, `run20332` — none of which populate a step-transition sidecar for the hub they resume) passing byte-for-byte unchanged.
- `V-4` Provider-agnostic by construction: the fix lives entirely in `resumedParentAgentAnnotations`, keyed on flow-node label and step-transition data, with no `providerKey` branch. Verified with an identical single-reviewer reinvoke shape run across Codex, Claude, and Grok in one table-driven test, plus a live re-check of all three real runs from the same test session.

### Constraints

- Backend-only; no desktop change — the fix only changes which timestamps/activation-count the server derives before emitting `agent_spawned_by_user`/`agent_result_injected` annotations.
- Does not change the live (non-restart) path at all — `reinvokeMatchingFlowChild` already emits a correct new card per round while the server process is up; this bug only affected reconstruction after a restart.

### Open Questions

- None for the defect. Whether the Drive-sync manifest should also carry the step-transition sidecar (mirroring what BUG-313 did for the turn log) so a Drive-restored chat gets the same precise activation timing cross-machine is a natural follow-up, not required for this fix — restore already falls back correctly to the turn-log heuristic when the sidecar isn't present.

### Source Refs

- `apps/local-runner/internal/runner/interactive_resume.go` — `resumedParentAgentAnnotations` (labelCounts guard + step-transition wiring), new `stepNodeActivationsFromLog`/`stepNodeActivationTimes`/`stepNodeActivation`, pre-existing `resumeChildActivationCount`/`resumeActivationTimestamps`/`peerStartWaveTimes` (fallback path, untouched).
- `apps/local-runner/internal/runner/step_transition_log.go` — `stepTransitionLine`, `StepTransitionLogStore` (`LoadStepTransitions` returns `nil, nil` for a missing sidecar — the exact property the fallback relies on).
- `apps/local-runner/internal/runner/bug314_reinvoke_activation_from_step_log_test.go` — the 3 new tests (§8).
- Live evidence: Claude hub `run-53157` (Gate-sandbox) — coder child `run-53162` and reviewer child `run-53261`, each genuinely reinvoked once (2 activations total), restored with only 1 activation each before the fix; Codex hub `run-46797` (2 reviewers per round, 3 rounds) and Grok hub `run-45881` (single round) both already correct and used as the non-regression baseline.

## 1. Issue Summary

A Review Loop chat using exactly one reviewer agent (no parallel reviewer cohort) loses every round after the first when the chat is reopened following a server restart — the coder's and reviewer's later activations, which genuinely ran and produced real results (a commit, a change-requested verdict), simply do not appear as cards. The same flow shape with 2+ reviewers restores correctly.

## 2. Parent Links

- impacted coding plan: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — same "reconstruct the timeline after a restart" contract BUG-313 documents for Drive restore; this is the same-machine-restart half of that contract.
- impacted tech design: `none directly` — extends the Task-239 step-transition sidecar's use (previously only for `applyStepTransitionReplay`'s workflow-step-runtime replay) to the agent-card reconstruction path.
- impacted system spec: `none known`

## 3. Environment and Reproduction

- environment: any Review Loop (or similarly reinvoke-lifecycle) flow where a node reuses the same run id across 2+ rounds, AND no *other* child in the hub provides a second "peer start time" close enough in time to register as a separate wave — the single-reviewer case is the common trigger, but any flow shape lacking a same-round peer for the reused node hits it.
- reproduction steps:
  1. Run a Review Loop flow with one coder + one reviewer (both reinvoke-lifecycle) for 2+ rounds — e.g. the coder's fix is rejected once, coder re-submits, reviewer approves.
  2. Restart the server process (`node scripts/supervisor.js --restart-existing` or equivalent).
  3. Reopen the chat from history.
  4. expected: both rounds' coder and reviewer cards are present (matching what was on screen live before the restart).
  5. actual: only round 1's coder and reviewer cards restore; round 2 (and any later round) is silently gone, even though the hub's own synthesis prose for that round is still present.
- frequency: 100% for this flow shape; 0% for flow shapes where the reused node has a same-round peer (e.g. Codex's 2-reviewers-per-round shape, confirmed unaffected both before and after this fix).

## 4. Expected vs Actual

- expected: reopening a chat after a server restart reaches parity with what was on screen live — every round's agent cards, not just the first.
- actual: parity held only for hub-level synthesis prose (driven by the turn-log's `transcript_turn` frames, already correct per BUG-313's investigation); the per-child agent-card count came from a different, narrower heuristic that silently degraded to "1" whenever no peer child supplied a corroborating wave signal.

## 5. Root Cause

- hypothesis: operator asked whether this was Claude-specific, since Codex and Grok restored fine in the same test session.
- confirmed cause: not provider-specific — flow-shape-specific. `resumedParentAgentAnnotations` (interactive_resume.go) determines how many agent-card "activations" a resumed child should get via `resumeChildActivationCount`: `max(session.TurnCount, count of turn-log prompt lines)`. For the Claude coder (`run-53162`), this correctly computed 3 — but only 2 of those 3 turn-log prompts represent a real reinvocation; the middle one is the flow gate's own mid-round reprompt ("missing change-audit note"), which the coder answered within the *same* round without the flow engine ever reinvoking the node. The subsequent cap — `if waves := peerStartWaveTimes(peerStarts, child.startedAt, 45*time.Second); len(waves) > 0 && activations > len(waves) { activations = len(waves) }` — exists to correct exactly this kind of over-count, but it derives its correction signal from *other children's* start times clustering into waves (one wave per round). With only one reviewer in the flow, `peerStarts` (excluding the coder's own start) contains a single value, which trivially forms exactly one wave — collapsing both the coder's and the reviewer's activation count to 1, discarding every later round regardless of how many genuinely happened.
- evidence: live inspection of `run-53157`'s sidecar files — `run-53157-step-transitions.ndjson` shows unambiguous `my-coder`/`my-reviewer-claude` `RUNNING`→`DONE` pairs twice each (02:51:44→02:53:30 and 02:55:12→02:55:28 for the coder; 02:53:31→02:54:46 and 02:55:29→02:55:44 for the reviewer) — ground truth for 2 real activations each, which the pre-fix heuristic could not recover without a peer wave. All 4 of the new red-first tests (§8) fail on pre-fix HEAD with the exact symptom (`coder round-2 card lost on restart: spawns = 1, want 2`).

## 6. Fix Strategy

- `F-1` Add `stepNodeActivationsFromLog`/`stepNodeActivationTimes` (interactive_resume.go): walk a hub's step-transition lines for one node id in order, pairing each `RUNNING` with the next terminal (`DONE`/`FAILED`/`CANCELED`) transition — one pair per real activation, independent of turn-log prompt counting or peer timing.
- `F-2` In `resumedParentAgentAnnotations`, compute `labelCounts` (how many distinct child runs claim each label) once per hub. When a label has exactly one claimant, prefer `stepNodeActivationTimes` for that child's activation count/timing; otherwise (no step-transition sidecar, or the label is legitimately shared across several spawn-lifecycle children) fall through unchanged to the pre-existing `resumeChildActivationCount` + `peerStartWaveTimes`-capped `resumeActivationTimestamps` path.
- `F-3` No change to the live (non-restart) reinvoke path (`reinvokeMatchingFlowChild`), to `applyStepTransitionReplay`'s own use of the same sidecar for workflow-step-runtime status, or to any existing resume-order test's setup.

## 7. Validation

- `V-1` Red-first TDD: `TestBug314SingleReviewerReinvokeRestoresBothRoundCards` and the 3 `TestBug314ReinvokeCardsSurviveRestartForEveryProvider` subtests (codex/claude/grok) all fail on pre-fix HEAD with the exact defect (`coder round-2 card lost on restart: spawns = 1, want 2` / same for the reviewer), proven via `git stash` isolating only the `interactive_resume.go` fix (new test file kept in place) — then pass after popping the stash back.
- `V-2` `TestBug314SpawnLifecycleMultiRoundReviewersStayOneCardPerChild` locks in the `labelCounts` guard: a Codex-shaped hub with 2 reviewer labels reused across 3 rounds (fresh run id each round, step-transition log has 3 `RUNNING`/`DONE` pairs per shared label) must still restore exactly 1 card per distinct child run, not 3 — passes both before and after the fix (it is a forward-looking regression lock for this fix, not a repro of a pre-existing bug, since the already-correct Codex/Grok behavior predates this change).
- `V-3` `go build ./...`, `go vet ./internal/runner/...` clean. Targeted sweep (`TestBug314*`, `TestRun5695*`, `TestRun9034*`, `TestRun1264*`, `TestRun20332*`, `TestRun24377*`, `TestBug294*`, `TestRun12613*`) — 37/37 pass, including every pre-existing resume-order/wave/cluster test unchanged.
- `V-4` Full `go test ./...` on the fix: 2486 passed, 16 failed (all catalogued pre-existing machine/CLI-dependent failures — real `codex`/git-shim/Windows-path/skills-fixture tests, none touching chat-sync or resume). Same sweep on `git stash`-isolated baseline (fix removed, new tests excluded via `-skip`): 2479 passed, **17** failed — the same 16 plus one extra (`TestStartTurnGrokSameAccountFollowUpUsesPromotedRealID`, a Windows `TempDir` cleanup race in an unrelated Grok-account file, confirmed by file location to touch none of this fix's code) — confirming the fix introduces zero new failures.
- `V-5` Live re-verification on the running dev runner (project Gate-sandbox), after rebuilding and restarting via `node scripts/supervisor.js --restart-existing`: resumed all three real hubs from the operator's own test session.
  - `run-53157` (Claude, the broken one): `admin/workflow-runs/run-53157/events` now shows `agent_spawned_by_user`/`agent_result_injected` for `my-coder`(`run-53162`) **twice** (02:51:44 + 02:55:12, second result correctly carrying "Committed as `2c70dfa`…") and `my-reviewer-claude`(`run-53261`) **twice** (02:53:31 + 02:55:29) — both previously-missing round-2 cards now present, matching the operator's own live screenshot.
  - `run-46797` (Codex, 2 reviewers × 3 rounds): unaffected — `reviewer_correctness`/`reviewer_security` still exactly 1 spawn per distinct child run id (6 children, 6 spawns); `coder` (reinvoke, sole claimant of its label) now sourced from the step-transition log too, still correctly 3 (matches its pre-fix count — the peer-wave heuristic happened to get the right answer here only because this flow has enough peer signal, unlike the Claude case).
  - `run-45881` (Grok, single round): unaffected — 1 spawn per child, unchanged.

## 8. Regression Guard

- tests: `apps/local-runner/internal/runner/bug314_reinvoke_activation_from_step_log_test.go` (`TestBug314SingleReviewerReinvokeRestoresBothRoundCards`, `TestBug314ReinvokeCardsSurviveRestartForEveryProvider`, `TestBug314SpawnLifecycleMultiRoundReviewersStayOneCardPerChild`).
- alerts: none.
- audit checks: [CA-408](../../change-audit/CA-408-reinvoke-activation-from-step-transition-log.md).

## 9. Follow-Up Document Updates

- upstream docs updated: [CP-51 Phase A/B Timeline And Verification Log](../../07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md) — new dated update appended alongside the existing BUG-312/BUG-313 notes.
- notes left unchanged on purpose: whether the Drive-sync manifest should also carry the step-transition sidecar for cross-machine activation-timing parity (mirroring BUG-313's turn-log fix) is a natural follow-up, not required — Drive restore already falls back correctly to the pre-existing heuristic when the sidecar is absent, same as any other pre-Task-239 run.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-314
change_type: bugfix
summary: resumedParentAgentAnnotations now derives a reinvoke-lifecycle node's restored activation count/timing from its durable step-transition RUNNING/DONE log when it uniquely owns that label, so a Review Loop flow with only one reviewer (no cohort to infer rounds from) no longer loses every round after the first when reopened post-restart.
# --->8---
