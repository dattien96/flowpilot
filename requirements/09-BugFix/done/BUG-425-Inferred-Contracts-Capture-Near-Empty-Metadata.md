# BUG-425: Inferred contracts commit near-empty metadata (no declared_paths, empty intent, bogus feature_key)

## Metadata

- Document ID: `BUG-425`
- Title: `Inferred contract committed from the reprompt turn's incremental diff, not the original code diff — feature_key guessed from noise ("claude"/"") and poisons the catalog`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-43-Test-Steps](../../07-Coding-Plan/done/CP-43-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp43/RESULT.md` (BUG-LIVE-CP43-001)
- Feature Keys: `change-contract`, `inferred-contract`, `feature-catalog`, `gate-reprompt`

## AI Quick View

### Summary

- When a turn is reprompted for `r-ca`/`r-contract`, the inferred contract row is committed from the **reprompt turn's incremental diff** (which contains only the new `change-audit/*.md` file) — not from the original turn's code diff.
- Live rows: `{"feature_key":"claude","intent":"","confidence":"inferred"}` with **no `declared_paths` at all**, and a second row with `feature_key:""` (empty).
- The persisted contract is nearly useless for scope gating and **poisons the feature catalog** — the bogus `claude` feature with whole-repo globs it registered is what later hijacked canonical-head injection (BUG-417 / CP43-003).

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

Inferred `contracts.ndjson` rows carry `declared_paths` absent, empty `intent`, and a `feature_key` guessed from diff noise (`"claude"`, `""`) instead of the feature the code change actually touched.

### Expected

The inferred contract is derived from the original turn's code diff — real `declared_paths`, an intent summary, and a feature key resolved against the actual changed files (or the run's declared contract).

### Actual

- grok run-656 turn-658 (code change, no `[Change Contract]`) → one `r-ca r-contract` reprompt → reprompted turn-811 → contract committed as `{"run_id":"run-656","feature_key":"claude","intent":"","confidence":"inferred"}` — no `declared_paths`.
- grok run-6404 turn-6549 reprompt → inferred row with `feature_key:""` (empty string).
- Devin path (run-486/turn-488): `r-contract`/`r-ca` never fire at all because `WrittenPaths` stays empty (known BUG-LIVE-CP35-002) — an inferred contract was still silently committed with no nag.

### Impact

- The `r-contract` remediation "works" mechanically (exactly one reprompt, non-blocking) but produces contracts useless for scope gating.
- Catalog pollution: the committed row registers a `claude` feature with whole-repo globs, which then dominates `ResolveFeature` and hijacks canonical-head injection into coder prompts (see BUG-417).

## Reproduction

1. On grok (or any provider whose `WrittenPaths` populates): run a turn that changes code without a `[Change Contract]` block → `r-ca`/`r-contract` reprompt fires.
2. The reprompted turn writes only `change-audit/*.md`.
3. Inspect `contracts.ndjson` → inferred row has no `declared_paths`, empty intent, noise `feature_key`.

## Root cause

- Contract inference runs on the reprompt turn's incremental diff — which contains only the change-audit note — instead of the original turn's code diff; `declared_paths` derives from an empty/codeless diff and `feature_key` is guessed from noise tokens.
- (Located in the contract-commit path of the reprompt handler — `apps/local-runner/internal/runner` gate reprompt → inferred contract write; see `contracts-so-far.ndjson` rows 2-3 and `runner.log:1237-1244, 1609-1612, 17321`.)

## Evidence

- `~/fp-beds/lt-evidence/cp43/RESULT.md` — BUG-LIVE-CP43-001: `contracts-so-far.ndjson` rows 2-3 (`feature_key:"claude"` and `""`), `l432-grok-events.json` seq 151-152, `runner.log:1237-1244, 1609-1612, 17321`.
- Downstream damage documented in `~/fp-beds/lt-evidence/cp43/RESULT.md` BUG-LIVE-CP43-003 and `~/fp-beds/lt-evidence/cp37/RESULT.md` BUG-LIVE-CP37-003 (→ BUG-417).

## Severity

- `medium` — contracts exist but are content-free, and each one can register a poison catalog feature; enforcement loop itself functions.

## Completion Notes (implemented 2026-09-23, CA-928b)

- Root cause confirmed: every `startTurn` re-captures `turnStartGitHead`,
  so a gate-reprompt turn's `observeTurnScopedDiff` legitimately contains
  only the remediation delta (the `change-audit/*.md` note). The root gate
  never populated/read `pendingGateCodePaths`, and `prepareChangeContract`
  inferred from that near-empty diff → `declared_paths` absent, empty
  intent, noise `feature_key`.
- Fix (`internal/runner/gate_hook.go`): the failing turn's paths are now
  stashed onto the durable `pendingGateCodePaths` carrier when the root
  gate queues a reprompt (union, survives chained reprompts); on the
  reprompt turn they are folded back into `changedPaths` (feature-key
  suggestion) and into the diff passed to `prepareChangeContract` via new
  `mergeCarriedPathsIntoDiff`. `tr.GitDiff` deliberately stays turn-scoped
  so `r-tests`/`r-reg` are not re-fired on the old change. Carry is cleared
  on gate pass (clean and warn paths). Child gate got the same merge and
  union-stash.
- Carrier durability: `pendingGateCodePaths` was already serialized in
  `local_file_session_store.go` and rehydrated in `interactive_resume.go` —
  the carry survives runner restart / reprompt replay, no schema change.
- Provider parity: shared post-finalize gate path, provider-neutral inputs
  (`fin.ChangedFiles`, observed diff). Devin's empty `WrittenPaths` case
  falls back to diff-derived paths.
- Tests added: `internal/runner/bug425_gate_reprompt_contract_paths_test.go`
  — unit merge semantics + enforce-mode reprompt stash + end-to-end
  reprompt-turn inference (committed contract carries `src` declared_paths
  and `feature_key=calc-core`, carry cleared on pass).
- Suite note: full `internal/runner` run compared against baseline; the
  known pre-existing failure set is unchanged.
- Live verification (2026-09-23, `/tmp/fp-live-425`, persistent runner :4468,
  opencode `google/gemini-3.6-flash`, enforce gate mode):
  - turn-63 wrote `src/mul.go` → gate fired `r-newtest` reprompt; the
    reprompt turn's own observed diff contained only `src/calc_test.go`
    (`mul.go` was already dirty at reprompt start — the exact incremental-
    diff mechanism from the report).
  - Committed `contracts.ndjson` row for run-23:
    `{"feature_key":"flowpilot","intent":"","declared_paths":["src"],"confidence":"inferred"}`
    — `declared_paths` covers the real change; `feature_key` resolved to a
    real catalog entry (workspace `flowpilot` feature), not provider noise.
  - Contrast row committed pre-fix shape (run-11, codex no-code turn):
    `{"feature_key":"","intent":"","confidence":"inferred"}` — no
    `declared_paths` at all.
  - Catalog inspection: no bogus `claude`/empty-key feature registered.
  - Provider availability note: devin auth timed out and grok returned 402
    (quota) on this machine, so the live path ran on opencode; the fix is in
    the provider-shared gate path and the devin `WrittenPaths`-empty case is
    covered by the `changedPathsFromDiff` fallback + unit tests.
