---
id: CA-929
title: Gate reprompt carry + inferred-contract metadata fixes (BUG-439, BUG-440)
type: BugFix
feature: flow-gates
date: 2026-09-23
status: done
---

## Context

- BUG-439: a reprompt-turn inferred contract could persist an empty `intent`
  and an unverified feature key (empty or catalog noise such as `claude`).
- BUG-440: `pendingGateCodePaths` carry could lose source paths — file-event
  gaps left the reprompt turn's contract without the real `src/` paths, and an
  LSP-driven reprompt cleared the carry before the recheck.

## Change

`internal/runner/gate_hook.go`:

- `prepareChangeContract` keeps inferred metadata honest: feature keys that did
  not resolve against the registered catalog are not persisted as truth, and
  head intent is stripped when committing an inferred contract so prose intent
  is not smuggled into `BehaviorStatement`.
- Root gate populates and reads the durable `pendingGateCodePaths` carrier
  (previously child-gate-only): reprompt stash is a union across chained
  reprompts, cleared on gate pass on both clean and warn/approve paths, and
  carried paths merge into `changedPaths`/`suggestFeatureKeys`/
  `prepareChangeContract` via `mergeCarriedPathsIntoDiff` without widening
  `tr.GitDiff` (so `r-tests`/`r-reg` do not refire on the old change).
- Child gate got the symmetric treatment: union stash, `tr.ChangedPaths`
  widened with `pendingPaths`, merged diff for contract prep.

`internal/runner/lsp_hook.go`:

- LSP gate check accepts carried paths (variadic) so an LSP-triggered reprompt
  re-evaluates the original turn's source paths, and the carry is restashed
  after LSP gate processing instead of being dropped.

## Tests (added only)

- `internal/runner/bug439_440_gate_contract_test.go` — six tests: carry merge
  unit coverage, enforce-mode reprompt stash, e2e inferred contract with real
  `declared_paths` + catalog-resolved `feature_key`, carry cleared on pass,
  LSP recheck keeping carried paths, and intent/empty-key non-persistence.
  All honestly red before the change (fixture had to disable `r-newtest`/
  `r-fk` defaults so the LSP rule was actually reached — no test-only pass).

## Result

- `go test -count=1 ./internal/runner -run 'Bug439|Bug440|Bug425'` — green.
- Provider parity: all changes are in the provider-shared gate path; no
  adapter/event/session code touched.

# ---8<--- flowpilot:change-ledger
feature_key: flow-gates
source_doc_id: BUG-439
change_type: bugfix
summary: Gate reprompt carry preserves source paths across LSP recheck; inferred contracts no longer persist empty intent or unverified feature keys
# --->8---
