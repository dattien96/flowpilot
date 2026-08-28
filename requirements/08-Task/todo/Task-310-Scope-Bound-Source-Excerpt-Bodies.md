# Task-310: Omit Source Excerpt From Coding Prompts

## Metadata

- Document ID: `Task-310`
- Title: `Omit Source Excerpt From Coding Prompts`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-28`
- Last Updated: `2026-08-28`
- Parent Documents: [CP-50 P-3](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md), [CP-43-CATALOG](../../07-Coding-Plan/done/CP-43-Context-Source-Catalog-And-Test-Log.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Child Documents: `None`
- Related Documents: [Task-246](../done/Task-246-Source-Excerpt-Runtime-Hint-Producers.md), [Task-168](../done/Task-168-Flow-Mode-Context-Package-Contract.md), [Task-259](../done/Task-259-Source-Dependence-Context-Source.md), [CA-655](../../../change-audit/CA-655-context-sources-default-and-frozen-fallback.md), [CA-656](../../../change-audit/CA-656-task310-scope-bound-source-excerpt-bodies.md), live F8 `run-174539`
- Replaces: `None`
- Tags: `source-excerpt, context-source, change-contract, prompt-budget, local-runner`

## AI Quick View

### Summary

- F8 tester/coder prompts (`run-174539`) spent ~88% of the shared Flow Context Package on `source.excerpt` full-file fences (`calc.go` plus dirty GitNexus fixtures outside scope).
- `### change.contract` already lists `declared_paths`. Dumping or even path-listing those files again is duplicate. Writers have `Read`/`Grep`.
- **Do not inject `source.excerpt` into coding prompts.** Keep Fetch/registry so package-level tests and freeze-chain `SourceExcerpts` stay intact (CP-55 `TestFirstCoderContextUsesCurrentFlowDeclaredPaths`).

### Current Ask

- Implement T-1…T-4. Additive tests only. Do not edit the legacy suite.

### Key Decisions

- `T-D1` **Omit at prompt compose, not at Collect.** `ComposeFlowCodingPrompt` / `ComposeFlowCodingPromptWithSecret` render a copy with `source.excerpt` stripped. `sourceExcerptSource.Fetch` and `defaultContextSourceIDs` stay (old tests + YAML parity).
- `T-D2` **No path-only list.** Declared paths are already in `change.contract`. Dirty extras are not in scope; listing them confuses writers.
- `T-D3` **Do not remove `source.excerpt` from the default set or harness YAML.** That would fail tests that assert the id is registered/defaulted (CA-655). Prompt omit is enough for F8 token cost.
- `T-D4` Stored `planContextPackage` may still contain excerpts; only the **provider prompt** drops them.

### Constraints

- Additive-tests-only + oracle-rule: new test file only. Do not edit `flow_context_package_test.go`, golden migration tests, `TestFirstCoderContextUsesCurrentFlowDeclaredPaths`, or `TestReadSourceExcerpts*`.
- Do not disable `source.excerpt` via CP-55 user settings.
- Provider-agnostic.

### Open Questions

- None. Operator locked: drop excerpt from prompts; contract already has declared_paths.

### Source Refs

- Live F8 `prompt-turn-174736.txt` / `prompt-turn-175056.txt`.
- `CP-50` P-3 / `Task-246` producers remain for package Collect.
- `SS-14` AC-9: missing excerpt in the prompt is not a failure.

## 1. Goal

Tester/coder (and any coding-step) prompts no longer contain `### Source: <path>` fences or excerpt file bodies. They still get `change.contract`, `source.dependence`, Head, history, discussion. Agents read files with tools.

## 2. Parent Links

- coding plan: [CP-50](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md) P-3, [CP-43-CATALOG](../../07-Coding-Plan/done/CP-43-Context-Source-Catalog-And-Test-Log.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- tech design: [SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md)
- system spec: [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) AC-9
- specific upstream ids: `Task-246`, `Task-168`

## 3. Trigger

Operator prompt-budget review of F8, then: path-only extras are redundant with `declared_paths`; drop excerpt entirely from the prompt.

## 4. Exact Change

Supersedes the earlier Approach B (scoped bodies + path-only extras) in this same task.

- `T-1` **`omitSourceExcerptFromPackage`** — copy of `FlowContextPackage` with `SourceExcerpts` nil and `source.excerpt` sections removed. Does not mutate the original.
- `T-2` **`ComposeFlowCodingPrompt` and `ComposeFlowCodingPromptWithSecret`** render `omitSourceExcerptFromPackage(pkg)` so live writer/handoff prompts never include `### Source:`.
- `T-3` **Do not change** `sourceExcerptSource.Fetch`, `readSourceExcerpts`, `defaultContextSourceIDs`, or harness YAML source lists.
- `T-4` **Additive tests** in `task310_source_excerpt_scope_test.go`:
  1. Coding prompt with contract + dependence + excerpt → contract/dependence present, no `### Source:`, no file bodies.
  2. Secret compose path also omits excerpt.
  3. Compose does not mutate the stored package's `SourceExcerpts`.
  4. `RenderFlowContextPackage` still dumps `### Source:` (package-level tests).
  5. `BuildFlowContextPackage` still Collects excerpts; compose omits them.

## 5. Touched Areas

- files: `flow_context_handoff.go` (`omitSourceExcerptFromPackage` + compose); `task310_source_excerpt_scope_test.go`; this task doc; CA-656
- modules: runner coding-prompt compose
- routes: none
- tables: none

## 6. Acceptance Check

- New T-4 tests green.
- Old tests untouched and green: `TestBuildFlowContextPackageSourceExcerptCapsAndOmissions`, `TestBuildFlowContextPackageRejectsOutsideWorkspacePath`, `TestRenderFlowContextPackageDependenceAfterContract`, `TestFirstCoderContextUsesCurrentFlowDeclaredPaths`, golden excerpt cases, `TestFlowCodingPromptIncludesPlanContextPackage`.
- Live F8 replay: tester/coder `prompt-turn-*.txt` has `### change.contract` and `### source.dependence`, **no** `### Source: calc.go` fence.

## 7. Out of Scope

- Removing `source.excerpt` from the registry or default set.
- Dedup Canonical Head vs current contract.
- Re-collect context after `test_signatures`.

## 8. Completion Notes

- result: **done 2026-08-28** — coding prompts omit `source.excerpt`; Collect/Fetch unchanged; CA-656 updated.
- follow-ups: optional catalog one-liner that coding prompts do not render `source.excerpt`.
- upstream docs updated: this task + CA-656.
