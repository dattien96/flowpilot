# BUG-426: `tournament-harness` entry spawn fails `workflow_has_no_steps` — all tournament node types dead code live

## Metadata

- Document ID: `BUG-426`
- Title: `Run materializes 6 step rows, but child entry spawn re-queries ListWorkflowSteps(pack-ref) → empty under file catalog → flow_start_all_entries_failed, will_reinvoke:false`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-65-Test-Steps](../../07-Coding-Plan/done/CP-65-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp65/RESULT.md` (BUG-LIVE-CP65-3)
- Feature Keys: `tournament-harness`, `flow-entry-spawn`, `workflow-steps`, `arbiter`, `merge`

## AI Quick View

### Summary

- `POST /client/flow-runs` with `FlowRef=flowpilot-core-flow-pack/tournament-harness` materializes 6 step rows (`problem_scout` FAILED, rest PENDING), but the entry-node child spawn re-queries `ListWorkflowSteps(workflowID)` for the pack ref — empty under the file catalog → `workflow_has_no_steps` → `flow_start_all_entries_failed` with `will_reinvoke:false` → run parks on `hub_stalled`.
- Consequence: no live `tournament.arbiter` verdict, no `retry` edge traversal (retry cap `max_attempts:2` unreachable), no `tournament.merge`/`WorktreeManager.MergeWinner`, no worktree cleanup — every tournament node type is dead code on this live path.
- Env-dependent root cause: the file-store catalog has no `workflow_steps` rows for the pack ref; a Supabase-mirrored environment may differ — needs re-test there.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

### Symptom

Tournament harness can never start its entry nodes live: `flow_start_entry_spawn_failed` / `flow_start_all_entries_failed` with error `workflow_has_no_steps`, `will_reinvoke:false`; the run then wedges on `hub_stalled`.

### Expected

The child spawn inherits a resolvable workflow/steps view — the 6 materialized step rows (or the pack flow definition) — and entry nodes dispatch; arbiter/retry/merge execute.

### Actual

- `run-1` steps-runtime shows the 6 rows materialized (`l65-1-steps-runtime.json`), yet `child_spawn_requested` → `workflow_has_no_steps` (`interactive_handlers.go:747-752` — `ListWorkflowSteps(in.WorkflowID)` returns empty for the pack ref the child inherits).
- Live side effects: multi-provider cohort ran (candidate-a devin, candidate-b opencode produced competing patches in worktrees) but judge/merge never ran; tie-card continue is accepted then silently discarded (hub re-escalates "no task artifact…"); escalation children 409 `session_unavailable` (sibling defects CP65-1/2/4 — see RESULT.md).
- Retry cap exists only inside `tournament.arbiter` (`DecideTournamentAction`, `max_attempts:2`, status `retry` → `when:retry` edge, `tournament_behavior.go:391-397`) — unreachable; verified by unit tests only.

### Impact

The entire tournament feature (arbiter ranking, retry ≤2, winner merge, worktree cleanup) is unexercisable live — CP-65 ACs for live arbiter verdict / winner record / merge are unverifiable; parked runs require manual recovery.

## Reproduction

1. `POST /client/flow-runs {"flowRef":"flowpilot-core-flow-pack/tournament-harness", …}` against a file/fake-catalog runner.
2. Observe `flow-diag-run-1.ndjson`: `child_spawn_requested` → `flow_start_entry_spawn_failed` `workflow_has_no_steps` → `flow_start_all_entries_failed` `will_reinvoke:false` → `hub_stalled` park.

## Root cause

- `apps/local-runner/internal/runner/interactive_handlers.go:747-752` — entry spawn calls `stepCatalog.ListWorkflowSteps(ctx, in.WorkflowID)`; the child inherits the pack ref `flowpilot-core-flow-pack/tournament-harness` for which the file-store catalog has no `workflow_steps` rows → `422 workflow_has_no_steps`.
- `apps/local-runner/internal/runner/interactive_service.go:2996` — failure logged as `flow_start_all_entries_failed` with `will_reinvoke:false`; no fallback to the materialized step rows or embedded flow definition.

## Evidence

- `~/fp-beds/lt-evidence/cp65/RESULT.md` — BUG-LIVE-CP65-3: `flow-diag-run-1.ndjson` (`flow_start_entry_spawn_failed`, `workflow_has_no_steps`), `l65-1-steps-runtime.json` (6 rows materialized), `l65-4-retry-cap-analysis.txt` (arbiter unreachable), `l65-1-candidate-*` (cohort ran, no judge/merge).
- Verified on main worktree HEAD `435e336b`: `interactive_handlers.go:747-752` (`ListWorkflowSteps` → `workflow_has_no_steps`), `interactive_service.go:2996` (`flow_start_all_entries_failed`).

## Severity

- `medium` (high for the feature) — tournament node types are dead code on the live path; env-dependent root cause needs re-test under a Supabase-mirrored catalog.

## Completion Notes (implemented 2026-09-23, CA-923)

- Root cause (four layers): `createRun` required catalog `workflow_steps`
  rows so a pack-ref child hit `workflow_has_no_steps`; `parallel_rollout`
  had no dispatch case; the candidate cohort join reinvoked the hub instead
  of the declared shared inline target; and both join gates required
  `status == completed`, so a failed member vetoed the arbiter entirely and
  the surviving candidate's patch was silently dropped (arbiter/merge
  SKIPPED, live run-2).
- Fix: `createRun` synthesizes steps from the embedded pack when the catalog
  is empty or errors; rollout passes through to `spawnTournamentCandidates`
  with isolated `candidate-<node>` worktrees (HEAD fallback when no
  flow-start SHA); `tournament.arbiter`/`tournament.merge` are inline-
  dispatchable; `flowSharedInlineJoinTarget` resolves the shared
  forward-done target for the cohort regardless of member outcome and
  `tournamentJoinSatisfied` accepts any terminal member status. The locked
  join path reads topology off the run directly (the helper would
  self-deadlock under `s.mu`).
- Tests: `TestBug426_CreateRunPackRefSynthesizesSteps`,
  `TestBug426_RolloutPassthroughSpawnsCandidates`,
  `TestBug426_CandidateJoinDispatchesArbiterAndMerge`,
  `TestBug426_JoinTargetIncludesFailedMember`,
  `TestBug426_ArbiterJoinSatisfiedWithFailedMember` (red by assertion).
- Live: runs 1890/2500/3077/3589/4262 all launched via pack ref; candidates
  spawned with worktrees; `cohort_join_inline_dispatch` +
  `tournament_arbiter_decided` fired every round including rounds where
  candidate-a failed on provider unavailability — the arbiter ran
  retry → retry → escalate instead of being skipped.
