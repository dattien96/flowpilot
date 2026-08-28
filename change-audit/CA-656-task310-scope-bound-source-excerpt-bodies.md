# CA-656 — Omit source.excerpt from coding prompts (Task-310)

## What

`source.excerpt` dumped full file bodies into tester/coder prompts. F8 `run-174539` packed ~14.5k chars of identical excerpt (`calc.go` plus dirty GitNexus fixtures). Operator then locked: do not inject excerpt at all — `change.contract` already lists `declared_paths`; writers `Read` files.

An earlier slice of this task scoped bodies vs path-only extras. That was superseded in the same session: path-only duplicates contract and confuses scope.

## Why

Prompt budget. Contract already names in-scope files. Blast-radius stays in `source.dependence`. File bytes belong to tools.

## Fix

- `omitSourceExcerptFromPackage` — copy with `SourceExcerpts` cleared and `source.excerpt` sections dropped.
- `ComposeFlowCodingPrompt` / `ComposeFlowCodingPromptWithSecret` render that copy so live coding prompts have no `### Source:` fences.
- `sourceExcerptSource.Fetch`, `defaultContextSourceIDs`, and harness YAML **unchanged** so package-level tests and CP-55 freeze-chain (`TestFirstCoderContextUsesCurrentFlowDeclaredPaths`) still see excerpts on the stored package.

Provider-agnostic. No existing tests edited.

## Tests

Additive — `task310_source_excerpt_scope_test.go`:

- coding prompt keeps contract + dependence, omits `### Source:` and file bodies
- secret compose path also omits excerpt
- compose does not mutate stored package
- `RenderFlowContextPackage` still dumps excerpt (legacy package tests)
- `BuildFlowContextPackage` still Collects; compose omits

Prior CA intact: CA-655 (frozen contract fallback), Task-246 Fetch producers.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: Task-310
change_type: feature
summary: Omit source.excerpt from coding prompts; contract already lists declared_paths
# --->8---
