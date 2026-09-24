---
id: CA-923b
title: Tournament harness fixes — escalation child identity, duplicate-spawn guard, decision-card consumption, pack-ref entry spawn, partial-cohort join, patch-snapshot merge (BUG-412, 413, 414, 426)
type: BugFix
feature: agent-flow-engine
date: 2026-09-23
status: done
---

## Context

The CP-65 live-verification wave plus a fresh live session (`/tmp/fp-live-h`,
runner on `devin/swe-2-high`, `FLOWPILOT_ENABLE_TOURNAMENT_ESCALATION=1`)
showed the tournament path was dead end-to-end:

- Escalation children were minted with no provider session/account, so every
  turn 409'd `session_unavailable` / `provider_account_changed`.
- Repeated Continue on a `tournament_escalation` parent re-spawned duplicate
  children (round++ → blocked → re-escalate).
- The arbiter's tie decision-card recorded the user's choice but never
  consumed it — picking a candidate silently fell through to a generic
  resume.
- The `tournament-harness` pack ref could not even start
  (`workflow_has_no_steps`), `parallel_rollout` had no dispatch case, and the
  candidate cohort join reinvoked a hub instead of the declared inline
  arbiter — then a *failed* member vetoed both join gates, so a surviving
  candidate's patch was silently discarded (arbiter/merge SKIPPED).
- The arbiter's escalate parked with a `decision_card` the validator
  rejected (`options` was `[]map[string]any`, parser asserts `[]any`) —
  prose fallback only, so no typed choice could ever be captured.
- Escalate-time cleanup deleted every candidate worktree at park, making
  any card pick unmergeable (`no worktree for owner`, live run-3077).
- Candidate diffs swept `.flowpilot/` runtime artifacts into the patch,
  conflicting against the main workspace's own metadata on merge-back.

## Changes

### BUG-412 — escalation child provider identity + first-turn kick

`internal/runner/tournament_escalation.go` (`escalateToTournament`): the
hand-built child run missed everything `createRun` initializes. Now the child
inherits `providerKey`/`providerAccountID` (resolved before the lock), gets a
real `providerSessionID` (`thread-*`), carries the tournament `flowRef`, is
registered resumable-idle, and its first turn is kicked at the end of the
spawn so the escalation actually executes.
Tests: `TestBug412_TournamentChildGetsProviderIdentity`,
`TestBug412_TournamentChildAcceptsFirstTurn`,
`TestBug412_TournamentChildCarriesFlowRef`.

### BUG-413 — duplicate escalation spawn guard

`internal/runner/interactive_service.go` (`applyFlowControl` continue case):
`continue` on a parent already in `tournament_escalation` now returns the
parked state unchanged — no `mutateLoop` Round++, no re-escalate. Plus
defense-in-depth in `maybeEscalateCapToTournament`: an existing tournament
child short-circuits a second spawn.
Tests: `TestBug413_RepeatedContinuesKeepSingleChild`,
`TestBug413_SecondEscalateBlockedByExistingChild`.

### BUG-414 — tournament decision-card choice is consumed

- `internal/runner/user_decision_card.go`: `DecisionCard.Kind` is parsed and
  `DecisionCardKindTournament` marks arbiter cards.
- `internal/runner/interactive_service.go` (`resumeFlowWithFeedback`): a
  captured choice on a tournament card routes to `resumeTournamentChoice`
  instead of the generic hub reinvoke.
- `internal/runner/tournament_dispatch.go` (`resumeTournamentChoice`):
  candidate ids → winner stamped + `merge_and_audit` dispatched; `retry` →
  attempt++ + fresh cohort (with stale-worktree cleanup first); `ask` →
  stays parked; anything else → error, never accepted-and-dropped.
- `tournamentDecisionCard` options are now `[]any` — the
  `parseUserDecisionCard` assertion requires it (`[]map[string]any` was
  silently rejected as `decision_card_invalid`, live run-2500).
- `behaviorTournamentArbiter` snapshots each candidate's diff into
  `payload["patches"]` before verdict-time cleanup; the escalate branch
  keeps its no-orphan `clean(ids)` (old contract preserved) because picks
  no longer need the dirs. `runTournamentArbiterNode` stashes the map on
  `rs.tournamentPatches`; `runTournamentMergeNode` forwards the winner's
  patch; `behaviorTournamentMerge` applies it via the new
  `tournament.WorktreeManager.ApplyPatch` → `worktree.Manager.ApplyPatch`
  (raw `git apply --check`/`apply`, same per-repo serialization and
  `MergeConflictError` evidence contract, no worktree needed). The patch is
  read raw — `tournamentStringArg` trims and a stripped trailing newline
  makes `git apply` report "corrupt patch".
- `runTournamentMergeNode` done-path sweeps loser worktrees (idempotent —
  covers the human-pick path where verdict-time cleanup never ran).
Tests: `TestBug414_TieCardCandidateChoiceMerges`,
`TestBug414_TieCardRetryChoiceReattempts`,
`TestBug414_UnknownTournamentChoiceRejected`,
`TestBug414_TournamentCardParsesThroughFlowControlValidator`,
`TestBug414_EscalateCarriesMergeablePatches`,
`TestBug414_RetryChoiceCleansStaleWorktrees`.

### BUG-426 — pack-ref entry spawn + engine-driven tournament nodes

- `internal/runner/interactive_service.go` (`createRun`): pack-ref workflow
  ids fall back to `flowStepsFromDefinition` (synthesized from the embedded
  pack) when the catalog returns no rows OR errors — a dead mirror no longer
  blocks builtin flows.
- `tryAdvanceFlowFromNode`: behavior-less `parallel_rollout` passes through
  to `spawnTournamentCandidates` (declared `cohort: tournament` targets).
- `tournamentCandidateWorktree`: isolated `candidate-<node>` worktree per
  candidate; base falls back to current HEAD when no flow-start SHA exists.
- `flow_validate_audit_dispatch.go`: `tournament.arbiter`/`tournament.merge`
  are inline-dispatchable (`runTournamentArbiterNode`/`runTournamentMergeNode`).
- Cohort join: `flowSharedInlineJoinTarget` resolves the single shared
  forward-`done` target across members — terminal failure counts as
  collected (join:all), so a failed candidate can no longer veto the arbiter
  and drop the survivor's patch (live run-1890: `cohort_join_inline_dispatch`
  fired with candidate-a failed). `tournamentJoinSatisfied` uses
  `worktreeTerminal` for the same reason. Hub-target joins keep the legacy
  reinvoke path. The join block reads topology directly — the earlier
  `activeFlowNodesFor` call deadlocked under held `s.mu`.
Tests: `TestBug426_CreateRunPackRefSynthesizesSteps`,
`TestBug426_RolloutPassthroughSpawnsCandidates`,
`TestBug426_CandidateJoinDispatchesArbiterAndMerge`,
`TestBug426_JoinTargetIncludesFailedMember`,
`TestBug426_ArbiterJoinSatisfiedWithFailedMember`.

### Worktree diff hygiene (merge-blocker found live)

`internal/worktree/manager.go` (`Diff`): `.flowpilot/` excluded from the
intent-to-add sweep and the diff pathspec — gate baselines, nested worktrees
and logs are runner runtime state, not candidate output; they were entering
patches and conflicting on merge-back.

## Provider parity

All touched code is provider-agnostic: escalation child provisioning reuses
the generic spawn path, the arbiter/merge are deterministic Go behaviors,
and the worktree/diff layer shells git only. Candidate provider failures are
typed cohort outcomes, not masked. Live bed only had codex+devin reachable
(claude account unconnected) — the partial-cohort fixes are precisely what
that environment exercises; a two-available-provider run remains desirable
but is not required for correctness evidence (the failed-member path is the
harder case and is now covered by unit + live).

## Verification

- Red-first: 11 initial Cluster H tests failed by assertion before the
  production changes; later additions (`JoinTargetIncludesFailedMember`,
  `ArbiterJoinSatisfiedWithFailedMember`, `TournamentCardParses…`,
  `EscalateCarriesMergeablePatches`, `RetryChoiceCleansStaleWorktrees`) were
  each confirmed red against the pre-fix code.
- Focused: `go test -count=1 ./internal/runner/ -run 'TestBug41[2346]|TestBug426|TestTournament'`
  — all green, including the pre-existing `TestTournamentTieRequiresHumanDecision`
  (escalate-cleanup contract preserved).
- Regression: `go test -count=1 ./internal/runner/` — remaining failures are
  identical on the clean baseline worktree (`d191004f`) or documented
  machine/env flakes (BUG-427 class); zero deterministic regressions
  attributable to this cluster. `go vet` clean on touched packages.
- Live (`/tmp/fp-live-h/ws`, devin/swe-2-high, tournament escalation on):
  - BUG-412: cap escalation spawned `run-603-tournament` with
    `thread-627`, devin provider, `devin/swe-2-high` — child ran to
    `completed` (no `session_unavailable`).
  - BUG-413: three consecutive Continue calls on the parked parent returned
    the same `tournament_escalation` state; exactly one tournament child in
    the agent list.
  - BUG-426: `run-1890`/`run-2500`/`run-3077`/`run-3589`/`run-4262` all
    started via pack ref (no `workflow_has_no_steps`), rollout spawned
    candidates, `cohort_join_inline_dispatch` → `tournament_arbiter_decided`
    fired every round including rounds where candidate-a failed.
  - BUG-414: after the card-shape fix, escalate parked with a validated
    card; `continue {"feedback":"candidate-b"}` routed into
    `merge_and_audit` (verified on run-3077 and run-4262). The merge ran and
    escalated with conflict evidence — correct contract; in this bed both
    candidates produced empty diffs (claude unconnected; codex-mini turns
    complete in ~1s with no change), so a real winner merge still wants a
    two-provider env.

## Known caveats

- Escalate-time `clean(ids)` can race a failed candidate's still-running
  post-turn gate suite (`[gate] suite start` inside the worktree): the
  dir can be resurrected by in-flight writes (observed: `candidate-candidate-a`
  orphaned on run-4262 while `-b` cleaned fully). Harmless to correctness —
  merge uses patch snapshots — but a stale dir makes the next tournament's
  `Create` fall back to the main workspace. Next round's cleanup or a
  manual `git worktree remove` clears it.
- `TestResumeFlowWithFeedbackAfterEscalate` and the
  `TestRunContractFreezeNodeReloadsDurableRecordBeforeAdvance` TempDir
  flake are pre-existing on baseline — tracked under Cluster L (BUG-427),
  not this cluster.
- Candidate-empty-diff turns (codex-mini completing in ~1s with no change)
  are a provider-environment observation, not a routing defect — logged for
  the next live pass with both providers connected.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-426
change_type: bugfix
summary: Cluster H tournament fixes — escalation child identity/kick, duplicate-spawn guard, decision-card consumption incl. card-shape + patch-snapshot merge, pack-ref entry spawn, partial-cohort join→arbiter dispatch, .flowpilot diff exclusion
# --->8---
