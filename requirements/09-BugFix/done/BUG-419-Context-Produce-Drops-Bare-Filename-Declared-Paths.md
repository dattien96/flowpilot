# BUG-419: Mid-flow `context.produce` drops bare-filename `declared_paths` → `source.excerpt` empty

## Metadata

- Document ID: `BUG-419`
- Title: `extractPromptSourcePaths drops tokens without a slash — frozen contract's root-level declared files never excerpted; asymmetric vs freeze-chain seeding`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-44-Pluggable-Context-Source-Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), evidence `~/fp-beds/lt-evidence/cp44/RESULT.md` (BUG-LIVE-2)
- Feature Keys: `context-produce`, `flow-context-package`, `source-excerpt`, `change-contract`

## AI Quick View

### Summary

- Mid-flow `runContextProduceNode` seeds `hints.ExplicitSourcePaths` from `extractPromptSourcePaths(resultMessage)` — a prose tokenizer that drops tokens without `/` or `\` — so bare filenames like `calc.go` in the frozen contract JSON never survive, and the frozen contract record is never consulted.
- Result: `source.excerpt` produced `Excerpts:null, Omitted:null` even though `calc.go` (the bug site) existed — the reproducer's context package missed the change's own files.
- The sibling freeze-chain path seeds `rec.DeclaredPaths` directly → identical contracts yield different excerpt content depending on which runner path produces the package.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

Frozen contract declared `["calc.go","calc_divide_zero_guard_test.go"]`, yet the emitted package's `source.excerpt` section was empty — `calc.go` was not excerpted into the reproducer's context.

### Expected

`context.produce` honors the frozen contract's `declared_paths` (including root-level bare filenames) when seeding `source.excerpt`, identically on both the mid-flow and freeze-chain production paths.

### Actual

`source.excerpt` → `Excerpts:null, Omitted:null`; section rendered empty (elided at render). The bug site file was available on disk and was declared in the contract, but neither the prose tokenizer nor any contract read supplied it.

### Impact

Context-quality gap: the very files the contract declares as in-scope are omitted from `source.excerpt`. Correctness unaffected in this run (the reproducer found the file via tools), but downstream agents lose the cheapest grounding signal. Asymmetry between the two package-build paths makes behavior contract-shape-dependent.

## Reproduction

1. Run `bug-harness` on a workspace whose frozen contract declares root-level files (no slash in the path, e.g. `calc.go`).
2. Let the mid-flow `runContextProduceNode` path build the package.
3. Inspect `flow_context_package.sections[source.excerpt]` → empty despite the declared file existing.

## Root cause

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:186` — `runContextProduceNode` seeds `hints.ExplicitSourcePaths` from `extractPromptSourcePaths(resultMessage)` only; never reads the frozen contract's `DeclaredPaths`.
- `apps/local-runner/internal/runner/flow_context_hint_paths.go:39` — `extractPromptSourcePaths` drops tokens lacking `/` or `\` (`!strings.ContainsAny(tok, "/\\") → continue`), so bare filenames never survive; also requires a file extension.
- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:2070` (freeze-chain sibling) — seeds `rec.DeclaredPaths` directly → asymmetric excerpt content for identical contracts.

## Evidence

- `~/fp-beds/lt-evidence/cp44/RESULT.md` — §8 BUG-LIVE-2 (observed `Excerpts:null` on `fcp-26b3529d`), `frozen_contracts.ndjson` (declared `[calc.go, calc_divide_zero_guard_test.go]`), `fcp-event-run-11.json`.
- Verified on main worktree HEAD `435e336b`: `flow_context_hint_paths.go` contains the `ContainsAny(tok, "/\\")` drop and extension check; `flow_validate_audit_dispatch.go:189` builds the package from `node.ContextSources`/`hints` only.

## Severity

- `medium` — context-quality gap on a default source section; asymmetric between the two production paths; no crash.

## Completion Notes (implemented 2026-09-23, CA-924)

- Root cause: mid-flow `context.produce` populated `ExplicitSourcePaths`
  only from `extractPromptSourcePaths(resultMessage)`, which discards
  tokens without a path separator — contract `declared_paths` like
  `calc.go` never reached `source.excerpt`. The freeze chain already seeded
  `DeclaredPaths` directly, making the two paths asymmetric.
- Fix: new `mergeDeclaredSourcePaths` in
  `internal/runner/flow_context_hint_paths.go` merges contract
  `DeclaredPaths` verbatim (bare filenames kept) with the prose parse;
  `runContextProduceNode` uses it.
- Tests: `TestBug419_BareDeclaredFilenameProducesExcerpt`,
  `TestBug419_MergedDeclaredPathsRetainBareFilename` (red by assertion
  pre-fix).
- Live: `/tmp/fp-live-i` run-3221 — `source.excerpt` section carries the
  `calc.go` excerpt (full `Percent` body) into the plan_writer prompt.
