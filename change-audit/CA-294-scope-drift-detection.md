# CA-294 — Scope-Drift Detection (Task-185, CP-43 P-2)

## Scope

Implemented (partially — see caveat) [Task-185](../requirements/08-Task/todo/Task-185-Scope-Drift-Detection.md) (P-2 of [CP-43](../requirements/07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md)): after a code turn, flag any edit that lands outside the turn's [Change Contract](CA-293-change-contract-capture.md) (Task-184), at file level, with a nag when no contract was declared at all.

## Changes

- New `apps/local-runner/internal/changecontract/scope.go`: `ScopeDiff(c, diff, sp)` — set-difference `actual_touched \ declared_scope` (exact/directory-prefix/glob match against `Contract.DeclaredPaths`, doc/audit files excluded); `HighSeverity(ctx, sp, outPaths)` — true only when `structure.Available()` **and** at least one out-of-scope path has dependents (`structure.Provider.Dependents`).
- `flowgate/rules.go`: two new default rules, `r-contract` (`code_changed_no_contract`, `reprompt`) and `r-scope` (`edit_outside_declared_scope`, `warn`) — auto-seeded into any existing `flow-rules.json` via the pre-existing `MergeDefaultRules` (same mechanism Task-223 used for `r-artifact-output`, no migration needed). New `TurnResult` fields (`ContractDeclared`, `ScopeOutOfScopePaths`, `ScopeHighSeverity`) carry the pre-computed signals in, since `flowgate` cannot import `changecontract` (the reverse import already exists, for `ChangedFile`).
- `flowgate/evaluate.go`: `code_changed_no_contract` fires when code changed and no `[Change Contract]` was declared this turn; `edit_outside_declared_scope` fires with the offending paths, downgrading a configured `block` action back to `warn` whenever `ScopeHighSeverity` is false (SD-21 D-5/Q-2 — file-level truth alone never blocks).
- `flowgate/enforce.go`: remediation text for `r-contract`'s reprompt.
- `runner/gate_hook.go`: `captureChangeContract` now also runs `ScopeDiff`/`HighSeverity` (the latter only when there's actually something out-of-scope, to avoid an unconditional GitNexus probe every turn) and feeds the results into `TurnResult`.

## Caveat — not fully done

`SD-21 F-2` (a rename-only/formatter diff must not trip `r-scope` when structure resolves it to an in-scope symbol) is **not implemented**. `structure.Provider` has no diff-hunk→symbol mapping anywhere in this codebase, and building one would mean writing new AST/symbol-extraction code — exactly the cost `SD-21 D-2` explicitly avoids ("no AST, no code-graph build in v1"). Scope-drift here is file-path-only; `Contract.DeclaredSymbols`/`ScopeDiff`'s `outSymbols` exist as the extension point but nothing populates or consumes them yet. Task-185 stays `in_progress`, not `done`, pending this or an explicit decision to accept file-level-only for v1.

## Verification

- `go build ./...` clean; `go vet` clean on `changecontract`, `flowgate`, `runner`.
- `go test ./internal/changecontract/... ./internal/flowgate/...` — 144 passed (11 new `scope_test.go` cases + 6 new `contract_rules_test.go` cases; `TestDefaultRules`'s hardcoded rule count updated 9→11 for the two new rules).
- `go test ./internal/runner/... -count=1` — 1386 passed, 16 failed (identical pre-existing environment-flake baseline — Codex CLI resume, account-home, skills-merge, auth-workspace), 18 skipped. Zero new failures.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-185
change_type: feature
summary: flow gate now flags edits that land outside a turn's declared Change Contract (r-scope, file-level, warn by default, block only with GitNexus + dependents) and nags once when no contract was declared at all (r-contract) — symbol-level rename/format false-drift suppression is explicitly deferred, not built
# --->8---
