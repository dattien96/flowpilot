# CA-442 — CP-53 P-5 r-newtest Reprompt Rule (Task-277)

## Scope

Retroactive change-audit for **Task-277 / CP-53 P-5**: **`r-newtest`** reprompt when a turn touches production code without adding a **new** test file — aligns gate oracle with safe-fix / oracle-rule / additive-tests-only.

## Prior CA (intact)

- **CA-439** machine verdict gate complements this — verdict enforces review-loop done; r-newtest enforces test debt on new production behavior.
- **r-tamper**: editing old tests does not satisfy r-newtest (explicit test lock).

## Changes

- `flowgate/rules.go`: register `r-newtest` (reprompt, production_change_no_new_test trigger).
- `flowgate/evaluate.go`, `flowgate/enforce.go`: evaluate trigger + remediation text ("ADD new test file — never edit old tests to pass").
- `flowgate/observe.go`: `HasNewTestFileAdded()` — only git-added (`A`) `*_test.go` / `*.test.ts` count.
- Tests (additive): `flowgate/cp53_waiver_newtest_test.go` — prod-only fires, new test silent, docs-only silent, old test edit does not satisfy.

## Provider impact

**Provider-agnostic** — rule inspects git diff scope only.

## Verification

```bash
go test ./internal/flowgate/ -run 'TestCP53RNewtest' -count=1
```

## Out of scope / residual

- Production-path allowlist for generated/vendor dirs — refine if false positives appear; disable via `flow-rules.json`.
- Default action is reprompt (not always-block) per CP-53 R-1 cry-wolf mitigation.

## Commits

- `98701f6` — implementation

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-277
change_type: feature
summary: Task-277 P-5 r-newtest reprompt rule when production diff lacks a newly added test file with old test edits explicitly not satisfying
# --->8---
