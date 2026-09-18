# CA-876 — CP-65 P-1 Tournament Arbiter scoring engine (Task-368)

# ---8<--- flowpilot:change-ledger
feature_key: tournament-harness
source_doc_id: Task-368
change_type: feature
summary: new internal/tournament package — deterministic arbiter scoring 0.5 tests + 0.3 LSP + 0.2 blast, disqualify-on-regression, human-decision on tie/empty
# --->8---

## Why

CP-65 P-1 is the tournament's scoring brain: every later slice (worktree,
flow, escalation, E2E) consumes its verdict. Scoring must be 100%
deterministic Go — never LLM judgment — so winners are auditable and unit
tests can assert exact scores.

## Change

- **`internal/tournament/types.go`** (new): `CandidateResult` (aggregated
  metrics in, provider key as label only), `CandidateScore` (weighted
  breakdown + `Disqualified`), `TournamentVerdict` (winner + ranking +
  `NeedsHumanDecision` + reason).
- **`internal/tournament/arbiter.go`** (new): `TournamentArbiter.Score`
  (`S_tests` exact pass ratio, zero-total scores 0; `S_lsp` 1.0 at 0 errors
  decaying 0.1/error floored 0; `S_blast` via `BlastBucket` 0→1.0 / 1–3→0.7
  / 4–10→0.3 / >10→0.0; `BrokeExistingTests` forces Total 0 +
  `Disqualified`) and `Decide` (deterministic order Total→SBlast→ID,
  skips disqualified, exact tie or zero eligible → human, never random).
- **`internal/tournament/arbiter_test.go`** (new): 4 Task-368 signatures +
  1 extra all-disqualified case (AC-5).

## Tests

- 5/5 green: higher-pass-wins (exact 0.6/1.0 ratios, full-weight total),
  LSP penalty (1.0 vs 0.5), blast tiebreak, regression disqualification
  (Total 0, never winner), all-disqualified → human with ranking intact.
- `gofmt` clean, `go vet` clean.
- Providers: Case-1 agnostic — grep over arbiter.go shows zero
  flowgate/lsp/structure imports and zero ProviderKey branching (R2 via
  label-only field).

## Prior CA claims kept intact

- New feature `tournament-harness`; no prior CA touched, no pre-existing
  test modified (additive only). No production behavior of any existing
  flow changes (new package, no callers yet — wiring lands in P-3/P-4).
