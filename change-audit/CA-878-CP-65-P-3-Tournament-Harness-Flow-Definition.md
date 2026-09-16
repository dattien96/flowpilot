# CA-878 — CP-65 P-3 Tournament harness flow definition (Task-370)

# ---8<--- flowpilot:change-ledger
feature_key: tournament-harness
source_doc_id: Task-370
change_type: feature
summary: tournament-harness.yaml (4 nodes + retry back-edge) + tournament.arbiter/merge behaviors + node config parsing + 4 flow tests
# --->8---

## Why

P-1 (scorer) and P-2 (worktrees) are libraries; P-3 wraps them in a runnable
flow selectable from the picker (never default) with the approved retry
back-edge: tie/all-fail + attempts left → fresh spawns + distilled brief.

## Change

- **`flows/tournament-harness.yaml`** (new): problem_scout (delegate,
  read_only) → parallel_rollout (config: 2 R-1 candidates, serial) →
  candidate-a/b (delegate/spawn/cohort/join-all, coder persona, candidate
  prompt) → tournament_arbiter (auto_pick, max_attempts 2) →
  merge_and_audit → done; retry back-edge (exactly 1); escalate edges to
  ask_user; selectableIn [flow]. No agent.code node (sandbox writes only).
- **`prompts/tournament-candidate.md`** (new, manifest-registered).
- **`tournament/config.go`** (new): ParseTournamentConfig (defaults R-1,
  1–3 candidates, known providers, max_attempts [1,3]),
  DecideTournamentAction (merge/retry/ask), DistillFailureBrief.
- **`runner/tournament_behavior.go`** (new): behaviorTournamentArbiter
  (baseline worktree → per-candidate real suite + LSP/dependents probes →
  regression attribution → verdict → merge losers-cleaned / retry all-cleaned
  + brief / ask all-cleaned + card) and behaviorTournamentMerge
  (MergeWinner → done + audit-shaped payload; conflict → escalate with patch
  evidence). Statuses done/retry/escalate route the YAML edges. Suite runs
  once per candidate (no double execution). Mock seam: Payload
  candidateResults skips git (P-5 E2E).
- **Registrations**: pack.go FlowNode.Config + parse (additive, nil for old
  flows), behaviorAliases += tournament.* (load requires it), BehaviorID
  consts, builtin inline registrations, registry.yaml doc entries, manifest
  flow+prompt entries (mirror consistency holds).
- **Tests**: tournament/config_test.go (5), agentpack/tournament_flow_test.go
  (4 signatures: topology, configs incl. 3-accept, 2nd-attempt brief,
  no-3rd-attempt), runner/tournament_behavior_test.go (8: registry scope,
  injected decide paths, real merge/conflict, real `go test -json`
  counting, full live mini-round A-wins-B-disqualified-merged).

## Tests

- New: 17/17 green (tournament 10 incl. P-1/P-2, agentpack 4, runner 8).
- Old: full agentpack green; runner behavior/pack/topology/freeze families
  green. `TestRecoveredFlowReusesPersistedFrozenContract` flaked once in a
  combined run (infra noise, zero assertion output) then 3/3 green alone —
  no mechanism links additive pack/registry changes to frozen recovery.
- R1: 2 old inventory tripwires asserted exactly 12 flows; the 13th flow IS
  this approved feature → operator authorized 12→13 with CP-65 comment
  lines (established per-CP pattern), nothing else in old tests touched.
- R2: Case-3 — configs name claude/codex/grok and parse (proof in tests);
  glue takes no providerKey (grep); live 3-way + per-provider dispatch
  verified at P-5 E2E (mock turns per Task-372).
- gofmt/vet clean on touched files (repo has pre-existing unformatted
  files; diff verified additive-only: pack.go +15, registry +7/+7).

## Prior CA claims kept intact

- CA-876/CA-877 untouched (no P-1/P-2 edits; verdict/WorktreeManager APIs
  consumed as specified). Retry + no-orphan policies implemented verbatim
  (conflict evidence in card, dirs never kept).
- Follow-ups for P-5: (1) live rollout-controller dispatch for the
  behaviorless parallel_rollout node; (2) 3rd-candidate runtime expansion
  (parse-ready); (3) frozen-contract gate posture for worktree writes.
