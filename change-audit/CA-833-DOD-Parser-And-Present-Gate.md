# CA-833 — Task-330: deterministic DOD parser + r-dod-present reprompt gate

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-330
change_type: feature
summary: add ParseDefinitionOfDone parser (dod.go) and r-dod-present reprompt gate so every Task-*/BUG-*.md written in a turn must carry a Definition of Done checklist
# --->8---

## Why

CP-47 P-1/P-2: task/bug planning flows sometimes emit documents with prose-only completion criteria. Acceptance must be machine-checkable, not vibes. r-dod-present (reprompt-only, 0 LLM tokens, deterministic) nags the writer to add a `## Definition of Done` checklist before work proceeds.

## Change

- `flowgate/dod.go` (new): `DodStatus{Present, Total, Checked, OpenItems}`, `ParseDefinitionOfDone` (accepts `## Definition of Done` / `## DoD`, case-insensitive; `[x]`/`[X]` count as checked; indented checkboxes counted; heading with 0 checkboxes = missing), `MissingDodDocs` (WrittenPaths filter to Task-*/BUG-*.md, disk read under WorkspaceCwd, graceful skip on unreadable files).
- `flowgate/rules.go`: register `r-dod-present` (Scope step, Trigger `task_or_bug_doc_missing_dod`, Action reprompt) in `DefaultRules()`; extend `DocScopeRuleIDs()` with the new cheap tier-1 check.
- `flowgate/evaluate.go`: new `checkRule` case firing the violation with the offending doc paths.
- GitNexus impact: `DefaultRules` 8 impacted / LOW (direct: LoadRules, MergeDefaultRules), `checkRule` 4 impacted / LOW (direct: Evaluate).

## Tests

`dod_test.go` (5 parser tests) + `r_dod_present_test.go` (10 gate/merge/degradation tests incl. `TestMergeDefaultRulesAddsRDodPresent` covering legacy `flow-rules.json`). All 10 Task-330 §10 signatures implemented fully. `go test ./internal/flowgate/...` fully green; gofmt/vet clean.

Disclosed legacy-test touch: `TestDefaultRules` manifest extended append-only with `"r-dod-present"` — repo convention per Task-233 / CP-53-Task-277 / Task-260 (documented in the test comment); no behavioral assertion altered.

## Providers

Case 1 agnostic: pure-Go markdown parsing + rule evaluation, no LLM call, no provider adapter — identical behavior for Claude/Codex/Grok turns by construction.

## Prior claims intact

CA-695 (r-additive-tests tamper paths), CA-442 (r-newtest), CA-441 (waiver ledger) — untouched; this change only appends a new rule and a new trigger case.
