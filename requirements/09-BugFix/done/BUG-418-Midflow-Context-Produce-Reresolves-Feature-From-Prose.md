# BUG-418: Mid-flow `context.produce` re-resolves feature from planner prose instead of the declared contract key

## Metadata

- Document ID: `BUG-418`
- Title: `context.produce ignores turn featureKey and re-resolves from planner resultMessage — wrong feature 3×; plus malformed legacy contract row`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-54-Test-Steps](../../07-Coding-Plan/done/CP-54-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp54/RESULT.md` (BUG-LIVE-1 + additional observations)
- Feature Keys: `context-produce`, `flow-context-package`, `feature-resolution`, `change-contract`

## AI Quick View

### Summary

- In the *contract-declared-but-not-frozen* window (legacy `contracts.ndjson` row, no frozen record yet), mid-flow `context.produce` ignores the turn's `featureKey` and resolves the feature from the planner child's `resultMessage` via lexical prompt matching — planner prose mentioning unrelated feature/provider names wins the score.
- Confirmed 3× live: run-1560 (`calc-core` → `sandbox-meta`), run-2274 (`sandbox-meta` → `claude`, provider names "Claude/Codex/Grok" in intent scored highest), run-3131 (`calc-core` → `sandbox-meta`).
- Related observation: a malformed legacy contract row was persisted (grok run-587 — prose concatenated into `declared_paths[0]`: `"stringutil/reverse.goTask-54LT-B is done…"`), showing the strict-JSON planner contract is not enforced on the legacy submission path; an empty prompt correctly produced a `user_question_required` event (run-455, guardrail working — retained as evidence).

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

FlowContextPackage emitted by mid-flow `context.produce` carries the wrong `featureKey` whenever the planner's `resultMessage` contains tokens that out-score the real feature (provider names, "sandbox" wording inside a `calc-core` contract, etc.).

### Expected

`context.produce` trusts the turn's declared/frozen `featureKey` (contract row) — the feature for the package is the contract's, not a fresh lexical guess from prose.

### Actual

| Run | Flow | Contract feature_key | FCP feature | Cause |
|---|---|---|---|---|
| run-1560 | context-coding-review-synthesis (devin) | `calc-core` | `sandbox-meta` | planner `resultMessage` contained "sandbox" words outscoring calc tokens |
| run-2274 | task-harness (grok) | `sandbox-meta` | `claude` | intent contained provider names "Claude/Codex/Grok" |
| run-3131 | task-harness (devin) | `calc-core` | `sandbox-meta` | same mid-flow re-resolution path |

Additionally, `contracts.ndjson` accepted a malformed legacy row for run-587 (`declared_paths[0]` = `"stringutil/reverse.goTask-54LT-B is done…"` — prose concatenated into the path field).

### Impact

All downstream context sections (canonical head, ranked feature history, chat summary) are built for the wrong feature — the locus/ranking work of CP-54 is applied to a feature the contract never named. Confined to the contract-declared-but-not-frozen window; frozen-contract runs (run-17 `preflight_contract_freeze` → `ResolvedFeatureKey`) resolve correctly.

## Reproduction

1. Run a flow whose `context.produce` node executes after a legacy `contracts.ndjson` declare but before any freeze (e.g. `context-coding-review-synthesis` or `task-harness` with contract seed but no frozen record).
2. Have the contract-planner child emit a `resultMessage` mentioning unrelated feature/provider names.
3. Inspect the `flow_context_package` event → `featureKey` ≠ contract's `feature_key`.
4. Malformed-row repro: grok run-587 contract submission concatenated prose into `declared_paths[0]` — row persisted as-is (`artifacts/contracts.ndjson`).

## Root cause

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go` (`runContextProduceNode`, ~:186) — mid-flow path seeds feature/hints from `resultMessage` prose (`extractPromptSourcePaths` lexical matching) instead of the turn's declared/frozen contract `feature_key`; the frozen-contract path (`advanceFlowThroughFreezeChain`) does use `ResolvedFeatureKey`, so behavior differs by which runner path produces the package.
- Legacy contract ingestion path lacks strict-JSON validation → prose lands inside `declared_paths` (`artifacts/contracts.ndjson` run-587 row).

## Evidence

- `~/fp-beds/lt-evidence/cp54/RESULT.md` — BUG-LIVE-1 (repro table) + "Additional observations" (malformed contract row, run-455 `user_question_required`).
- `planner-outputs.txt` (contract-planner resultMessages per run), `fcps.md` (8 FCP blocks), `runner.log`, `artifacts/contracts.ndjson` (malformed row), `bug-live-evidence.txt`.
- Workaround observed live: terse provider-neutral planner output (run-2975) or a frozen contract (run-17) yields the correct feature.

## Severity

- `medium` — wrong-feature context packages in a live flow path; reproducible 3×; confined to the pre-freeze window.

## Completion Notes (implemented 2026-09-23, CA-924)

- Root cause: `runContextProduceNode` built hints from `resultMessage` prose
  only — `ResolvedFeatureKey` was never seeded — so feature resolution ran
  on planner prose and catalog noise instead of the run's contract; legacy
  contract rows with prose-concatenated path values were also accepted
  verbatim by `splitAndTrim`.
- Fix: `runContextProduceNode` loads `latestContractForRun` (legacy →
  frozen, the existing unified seam) and seeds
  `ResolvedFeatureKey = contract.FeatureKey`; `splitAndTrim` in
  `internal/changecontract/parse.go` drops malformed whitespace-joined
  path values.
- Tests: `internal/changecontract/bug418_parse_test.go`,
  `TestBug418_ProduceUsesContractFeatureKey` (red by assertion pre-fix).
- Live: `/tmp/fp-live-i` run-3221 — plan_writer prompt embeds
  `feature: calc-core` + `### change.contract` (Scope: calc.go,
  Confidence: declared) produced by the mid-flow `context` node.
