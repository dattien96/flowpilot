# CA-695 — Task-260 r-additive-tests gate + safe-fix-contract Chat Plan/Code pointer

## Problem

Skill `safe-fix-contract` is prompt-only — AI can ignore it unless explicitly mentioned. The runner only surfaced a synthetic `warn` `r-tamper` when a pre-existing test file was `M/D/R/C` (step still completed). Greenwash (edit test to keep suite green) was not hard-blocked and Plan posture could author a plan without knowing R1/R2/R3/history.

## Change

- **Gate** (`flowgate/rules.go`): new `r-additive-tests` (`pre_existing_test_edited`, `reprompt`, `gate_mode`-gated like `r-ca`). `evaluate.go` fires only on `TamperedTestPaths` non-empty (populated from `oracle.Tampered` = `IsTestFile` + `M/D/R/C` filtered by `test_overrides.json` `filepath.Base`). Pure `A` (new test file) never fires; no `GitDiff` fallback so overridden files do not re-fire.
- **Wire** (`runner/gate_hook.go:276`): `TamperedTestPaths: append([]string(nil), oracle.Tampered...)` (defensive copy). Removed hardcoded `r-tamper` `warn` append — single rule owns signal.
- **Remediation** (`flowgate/enforce.go`): `pre_existing_test_edited` cites `safe-fix-contract / additive-tests-only / oracle-rule`, lists tampered paths, instructs revert + new test file + fix prod, stop-and-ask if old test is wrong.
- **Chat init** (`runner/chat_posture.go` + `interactive_service.go:6503`): Chat `plan`/`code` (`runKind==chat && !flowEngineDriven`) auto-merges `{Name:"safe-fix-contract", Source:"builtin"}` into `SelectedSkills` before `promptPrep`; `scan`/`non`/`""`/Flow never inject; dedup case-insensitive. `injectSelectedSkills` emits pointer only (`- /safe-fix-contract → <path>` + 1-line desc) for all providers.
- **Docs**: `SD-20 §1 pipeline + §2.8 r-additive-tests` (replaces synthetic `r-tamper`, retains alias), `CP-58` Related Documents + integration note (Flow wiring deferred).
- **Comment fix**: `gate_hook.go:348` `default warn` → `default enforce` (see `readGateMode`).

## Tests (additive, no pre-existing edits)

- `flowgate/r_additive_tests_test.go` (11) — `DefaultRules`/`MergeDefaultRules`, `M`→fire, polyglot Kotlin, `A`→no fire, `D/R/C`, remediation, `DocScopeRuleIDs`/`ArtifactRuleIDs` exclusion.
- `flowgate/task260_oracle_override_test.go` (2) — `IsOverridden(filepath.Base)` clears `Tampered`, integration `RunOracle → TurnResult → Evaluate` no fire.
- `runner/chat_plan_code_safefix_inject_test.go` (8) — `plan`/`code` inject, `scan`/`non`/`flowEngineDriven`/`non-chat` no inject, dedup, provider-agnostic (Claude/Codex/Grok/Gemini/Opencode via shared `injectSelectedSkills`).
- `runner/task260_gate_wire_test.go` (6) — wire `oracle.Tampered → TamperedTestPaths` defensive copy, `enforce=reprompt` vs `warn=warn` (parity with `r-ca`), override clears wire, pure `A` no fire.

All green: `go vet ./internal/flowgate ./internal/runner` clean; `go test ./internal/flowgate -run TestRAdditive` PASS; `go test ./internal/runner -run TestTask260GateWire` PASS.

## Out of scope

Flow `preflight_contract_plan`/`plan_writer`/`plan_reviewer`/`cp_plan_writer` pointer and coding-child `DocScopeRuleIDs` inclusion — deferred to CP-58. Hub park while delegate child `RUNNING` — separate task.

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-260
change_type: feature
summary: Promote r-tamper warn to r-additive-tests reprompt (oracle Tampered) + auto-inject safe-fix-contract pointer on Chat Plan/Code for all providers
# --->8---
