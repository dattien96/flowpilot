# CA-880 — CP-65 P-5 Tournament E2E verification (Task-372) + CP-65 closeout

# ---8<--- flowpilot:change-ledger
feature_key: tournament-harness
source_doc_id: Task-372
change_type: test
summary: E2E winner-merge + tie-fail-closed on real temp repos with real suites; CP-65 DOD 5/5 evidenced
# --->8---

## Why

P-1→P-4 are proven slice by slice; P-5 proves the chain on real git repos
with real `go test` runs: score-win merge on a RED baseline (complementing
the P-3 green-baseline disqualification win) and the §7 tie fail-closed.

## Change

- **`runner/tournament_e2e_test.go`** (new, test-only): temp Go module with
  a genuine Add bug. Winner scenario (A fixes green, B wrong-red): arbiter
  done/winner-A → merge done → main carries fix, main suite really 1/1
  green, zero surviving worktrees, HEAD unmoved. Tie scenario (both stay
  red, equal shape): escalate + NeedsHumanDecision + nameless winner +
  2-entry ranking card + main byte-untouched + full cleanup. Suites real;
  candidate turns mocked by file writes; LSP/dependents stubbed (T-1 seam).

## Tests

- 2/2 green (30s + 22s — real suite executions). Full tournament +
  agentpack packages green. No production file touched in P-5 (R1 surface
  nil); P-4's 85-test core sweep + P-3 suites already green post-change.
- DOD evidence map: arbiter matrix → P-1 tests + E2E score-win; worktree
  isolate+clean → P-2 tests + E2E zero-survivor asserts; escalation on cap
  → P-4 tests; flow stable → P-3 topology + E2E chain; additive green →
  this run.

## Prior CA claims kept intact

- CA-876→879 untouched. All CP-65 policies hold end to end: retry design
  (default max_attempts=1 → tie went straight to human, no retry leg),
  no-orphan (both scenarios assert clean lists), deterministic scoring.
- Doc moves only (CP-65 + Task-372 → done/, link repairs); no code moves.
