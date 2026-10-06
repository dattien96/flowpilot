# BUG-642 — The scaffold-pre-existing gate re-fires forever on the cumulative-since-BaseSHA diff: a declared path modified by ANY prior sanctioned leg (or operator remediation) keeps tripping `vibe owner-debate mount cap` on every subsequent scaffold turn; adjudications do not stick and the sprint wedges

- **ID:** BUG-642
- **Severity:** Critical — once triggered, the run cannot advance on its
  own: each tdd/scaffold pass re-detects the same diff, tries to mount
  an owner-debate, hits `maxVibeDebateMountsPerSprint`, and re-parks on
  `cap`. Operator discharge (`agent-loop/continue`, `gate-decision`
  custom, `gate-decision suggest-requirement-change`) releases one
  iteration and the condition re-fires minutes later.
- **Status:** PARTIALLY FIXED — defect 1 (attribution window) fixed in
  CA-1227: `preLockWritten` paths are now attributed to the current turn
  via `unchangedSinceTurnStart` (turn-start fingerprint or
  `git diff --quiet turnStartGitHead`), fail-closed when no anchor
  exists. The live wedge shape — sanctioned remediation committed before
  the turn — can no longer re-fire. Defect 2 (durable resolved-violation
  ledger for adjudicated-but-uncommitted writes) is deferred: after the
  attribution fix it only matters when a sanctioned write is never
  committed, and an uncommitted locked-path write arguably SHOULD keep
  flagging until it lands as a commit. Revisit if a live run shows that
  shape. Regression tests:
  `bug642_scaffold_preexisting_cumulative_test.go`.

## Evidence chain (all live)

1. Gate reason, verbatim every time:
   `vibe owner-debate mount cap reached: Flow gate: the scaffold
   modified production file(s) that existed at contract freeze:
   core/security-rasp/src/main/cpp/CMakeLists.txt — restore them
   byte-for-byte …`
2. The file's current content is the canonical Task-044 split produced
   by **sanctioned coder remediation** (commits 23781fd + d99145c) that
   the operator already adjudicated (adj-22, adj-31) and that amended
   contract v7 declares. Restoring freeze bytes would regress AC-4.
3. `gate_hook.go` scaffold-turn check: a declared path whose diff
   status vs `rec.BaseSHA` is not `"A"` counts as
   `scaffoldPreExisting` — the diff is cumulative from the contract
   freeze commit, so a file legitimately rewritten by an earlier coder
   pass (or amended into scope mid-sprint) is indistinguishable from a
   fresh unauthorized scaffold write.
4. Operator discharges used: `agent-loop/continue` (adj-22/31),
   `gate-decision option=custom` (adj-32), `gate-decision
   option=suggest-requirement-change` (adj-33) — each clears the park,
   the next scaffold pass re-parks. `tournament_escalation_deduped`
   also fires because the zombie tournament leg never releases the
   dedupe claim (see BUG-639 observations).

## Root cause direction

Two defects compound:

1. **Attribution window is wrong.** The scaffold check diffs against
   `BaseSHA` (contract freeze), not against "files this leg's turn
   actually wrote" or a per-round baseline. A sanctioned historical
   modification lives in the diff forever. The fix is to scope
   `preLockWritten` to the current turn's writes (the runner already
   tracks tool-call writes for the parallel drift message — reuse it),
   or to rebase the record after an adjudicated scope change.
2. **Adjudications are not durable against the gate.** A
   `suggest-requirement-change`/`custom` gate-decision parks+discharges
   but records nothing the gate consults on re-evaluation — no
   sanctioned-paths set, no freeze rebase, no resolved-condition
   ledger. The cap should honor the most recent adjudication for an
   identical violation signature (same paths, same gate id) instead of
   re-parking.

## Fix direction

- In the scaffold-turn branch, compute `preLockWritten` from this
  turn's write set (or subtract paths present in a durable
  `sanctionedScaffoldWrites` set recorded by gate-decision).
- On `gate-decision`/`agent-loop/continue` discharge of a cap block,
  persist a resolved-violation record keyed by (gateId, path set) so a
  subsequent identical detection is a no-op log, not a re-park.
- Add a regression test: scaffold turn after adjudicated remediation
  must not re-raise `vibe_debate_mount_cap` for the same path.

## DeclaredPaths

- `apps/local-runner/internal/runner/gate_hook.go` (~1150–1210,
  scaffoldPreExisting check)
- `apps/local-runner/internal/runner/vibe_gate.go`
  (`escalateVibeDebateMountCap`, `maybeEscalateCapToTournament`)
- `apps/local-runner/internal/runner/interactive_resume.go` or
  decision-persistence layer (resolved-violation durability)
