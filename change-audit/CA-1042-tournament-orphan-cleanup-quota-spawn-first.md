# CA-1042 — Tournament spawn-abort orphan cleanup + quota respawn spawn-first ordering (BUG-533, BUG-534)

## Summary

Deep review of the BUG-522..530 batch surfaced two important defects:

- **BUG-533**: `spawnTournamentCandidates` only cleaned created worktrees
  on the worktree-creation-failure branch. Provider-check, account-check,
  and `spawnChildRun` aborts left `candidate-*` worktrees on disk —
  permanently wedging retries behind "worktree already exists" (seen live:
  run-1504→run-1 needed manual `git worktree remove`).
- **BUG-534**: `respawnChildOnRoute` closed + persisted the old leg and
  `commitQuotaRotation` emitted `quota_route_committed` BEFORE
  `spawnChildRun`. A refused spawn (parent loop blocked on a decision
  card, BUG-432 — live run-1269) left a durably committed route with a
  closed leg and no successor, with nothing to replay it.

## Changes

- `internal/runner/tournament_dispatch.go` (BUG-533): `spawned` map tracks
  successful child spawns; `cleanupCreated()` sweeps `created − spawned`
  worktrees on every abort path — provider `Selectable` failure, missing
  connected account, `spawnChildRun` refusal, non-advancing parent break,
  and unattempted tail candidates. Planted/crashed stale dirs still hit
  the sweep+retry branch (unregistered dir → `os.RemoveAll` fallback in
  `WorktreeManager.Cleanup`).
- `internal/runner/quota_gate.go` (BUG-534): `respawnChildOnRoute` spawns
  the successor FIRST, then closes + persists the old leg.
  `commitQuotaRotation` emits `quota_route_committed` only after the
  respawn returns nil — no committed record for an unexecuted respawn.
  A refused spawn leaves the vetoed leg `active` so the next admission
  re-enters the gate and fires a fresh card.
- `internal/runner/interactive_handlers.go`: `createRun` bumps the minted
  id past any resident run before insert — latent collision exposed by
  spawn-first ordering (event ids and run ids share `idCounter`;
  emit-before-spawn used to consume a tick and mask it).
- Tests: `TestBug533_AbortCleansCreatedCandidateWorktrees`,
  `TestBug533_StaleDirIsSweptAndSpawnProceeds`
  (`review_followup_tournament_test.go`),
  `TestBug534_RefusedRespawnKeepsLegOpen` (`review_followup_durability_test.go`).
- Docs: `requirements/09-BugFix/todo/BUG-533-*.md`, `BUG-534-*.md`;
  runbook R8 section in `CP-Full-Live-Test.md`.

## Red test

- `TestBug533_*`: pre-fix, abort left `candidate-candidate-a/` orphaned and
  a planted stale dir blocked `Create` — both failed on assertion.
- `TestBug534_RefusedRespawnKeepsLegOpen`: pre-fix, the leg landed
  `leg_state=closed` with a `quota_route_committed` event while the spawn
  was refused — the exact wedge.

## Verification

- `go test -count=1 ./internal/runner` focused batch (BUG-533/534/516/517
  + tournament/quota review-follow-ups): PASS. Two transient suite flakes
  (`TestBug516` TempDir race, `TestBug517` id collision) — 517 was real
  and fixed via the `createRun` bump; 516 passes isolated.
- `go test -race` on the touched tests: PASS; `go vet`: PASS.
- `internal/worktree`, `internal/flowgate` packages: PASS.
- Broad runner suite: 15 failures, all classified — 12 environment
  baselines (missing `opencode`/`codex`/`agy`/`claude` binaries, MCP
  network), 2 pass isolated (suite timing/TempDir flakes). Not caused by
  this change.

## Live evidence (/tmp/fp-live3, runner :4319, tournament run-2634)

- BUG-533: stale `candidate-candidate-a/` dir (marker, not a git worktree)
  planted pre-run → spawn swept it, created a registered worktree,
  candidate ran. Retry cohort (attempt-1) swept prior remains again.
- BUG-534 refusal: `use_for_run|devin|…` answer on run-1408 (manual park)
  and run-3107 (organic gate-escalate park) → `respawn child: parent run
  "run-2634" loop is blocked (escalate)`; leg stayed `active`, zero
  committed events, no successor.
- BUG-534 re-admission: new turn on the open leg re-entered admission →
  veto refired → card `q-4146`.
- BUG-534 success: answering `q-4146` while running spawned `run-4150`
  (devin/swe-2-high, same label/cohort/worktree) → then run-3107 closed
  (`provider_switch`) → then `quota_route_committed` evt-4155 seq 3.

## Provider parity

Runner-core ordering + worktree lifecycle — provider-agnostic. Live
covered grok (vetoed leg) → devin (successor). The refusal contract is
provider-independent (`spawnChildRun` parent-guard).

## Follow-ups / known limits

- `quota_route_committed` is not in the CP-41 sidecar whitelist
  (`isFlowSidecarEventType`) — pre-existing; the committed repin survives
  via session rows (`leg_state`, successor row), not the event file.
- Re-bind loop residual from R6 stands: binding sees connectivity, not the
  durable quota block — a ledger-blocked account can be re-picked and veto
  again (fail-closed, one card per attempt).
