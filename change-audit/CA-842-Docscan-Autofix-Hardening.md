# CA-842 — harden docscan autofix idempotence and rule-slice aliasing

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-332
change_type: bugfix
summary: independent-review hardening — AutoFix no longer synthesizes a duplicate skeleton for canonical sections that exist before the Metadata block (scanner-clean docs stay byte-identical) and DefaultConformanceRules hands every rule its own Phases slice
# --->8---

## Why

Fresh-eyes review round 1: (1) a scanner-CLEAN doc whose `## 1. Goal` block sits before `## Metadata` still gained a redundant `## 1. Goal` skeleton from AutoFix (regionStart excluded the pre-Metadata block, so the rank looked missing) — contradicting the byte-for-byte-identity contract; (2) DefaultConformanceRules shared one backing `all` slice across 8 rules, so a mutating caller would alias them all.

## Change

- `docscan/autofix.go`: new preRegionCanonicalRanks; sectionFixNeeded and rebuildSectionRegion accept it — pre-region canonical sections satisfy the completeness check (no needless rebuild) and get their number slot reserved without a duplicate skeleton when a rebuild does happen.
- `docscan/rules.go`: per-rule freshPhases() copies.

## Tests

`docscan_test.go`: byte-identity test built from the real FORMAT-REFERENCE-TASK with its Goal block hoisted above Metadata (scanner-clean in, byte-identical out, rescan 0 issues); aliasing test asserts no two rules share a Phases backing array. Full docscan suite green (26 tests).

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-834 — FORMAT-REFERENCE sync (0 issues) and perf budgets re-verified by the suite.
