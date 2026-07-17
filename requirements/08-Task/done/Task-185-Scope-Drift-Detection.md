# Task-185: Scope-Drift Detection

## Metadata

- Document ID: `Task-185`
- Title: `Scope-Drift Detection`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-17` (file-level scope-drift closed; symbol-level F-2 + E-4 ignore set waived — see §8)
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-2), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-7)
- Child Documents: `None`
- Related Documents: [Task-184: Change Contract Capture](./Task-184-Change-Contract-Capture.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [Task-098: GitNexus Structure Provider](./Task-098-GitNexus-Structure-Provider.md)
- Replaces: `None`
- Tags: `scope-drift, flowgate, gitnexus, structure, local-runner`

## AI Quick View

### Summary

- Add flow-gate rules `r-contract` (no declaration) and `r-scope` (edit outside the declared contract), evaluated from the gate's existing `TurnResult.GitDiff`.
- Drift = a deterministic set difference `actual_touched \ declared_scope`; file-level always, symbol-level only when `structure.Available()` (GitNexus).
- Warn-first: `r-scope` blocks only when structure is available and configured; deliberately avoids the AST-hashing that got `SD-17 D-11` deferred.

### Current Ask

- **Done (2026-07-17):** file-level `r-contract`/`r-scope` + `ScopeDiff` wired and tested. Symbol-level rename/format false-drift (`SD-21 F-2`) and `SS-14 E-4` per-project ignore set **waived** for this task (see §6/§8).

### Key Decisions

- `T-1` Scope drift is a set diff over `GitDiff`, not a graph diff (`SD-21 D-2`).
- `T-2` `r-scope` = `warn` default; escalates to `block` only under `structure.Available()` + config (`SD-21 D-5`, resolves `Q-2`/`Q-3`).

### Constraints

- Reuse `observe.go` `TurnResult.GitDiff` and `enforce.go` action ladder (no new hook point); depends on Task-184's stored `Contract` and Task-098's `structure` provider.
- Exclude `requirements/`, `change-audit/`, `*.md`, and the `SS-14 E-4` per-project ignore set from the diff.

### Open Questions

- Should `r-scope` ever block at file level when structure is absent (`SD-21 Q-2`)?

### Source Refs

- `SD-21 §4`, `§6` (rule table `r-contract`/`r-scope`), `§7` steps 4–6, `§8 F-2`. `CP-43 §4.2`. `SS-14 US-3`, `AC-7`, `E-4`.

## 1. Goal

After a code turn, flag any edit outside the declared Change Contract, at file level always and symbol level when GitNexus is present, without false-drift on rename/format.

## 2. Parent Links

- coding plan: `CP-43` P-2
- tech design: `SD-21` §6 (rules), §7, D-2, D-5
- system spec: `SS-14` US-3, AC-7, E-4
- specific upstream ids: `P-2`, `r-contract`, `r-scope`

## 3. Trigger

Task-184 records intended scope; this task is the "flag when it changes anything else" half of `US-3`. The gate already observes the diff post-turn, so the check is a cheap set difference there.

## 4. Exact Change

- `T-1` `flowgate/rules.go` — add `r-contract` (`trigger:code_changed_no_contract`, `action:reprompt`) and `r-scope` (`trigger:edit_outside_declared_scope`, `action:warn`); seed both into `settings/flow-rules.json`.
- `T-2` `changecontract/scope.go` — `ScopeDiff(c Contract, diff []ChangedFile, sp structure.Provider) (outPaths []string, outSymbols []string)`: `actual = {f.Path}`; `outPaths = actual \ glob(c.DeclaredPaths)` minus doc/ignore set; when `sp.Available()`, map changed hunks → symbols and diff against `c.DeclaredSymbols`.
- `T-3` `flowgate/evaluate.go` — add the two triggers: `code_changed_no_contract` = code file in diff AND `changecontract.Store.Get(run,step)` returns none; `edit_outside_declared_scope` = `len(ScopeDiff.outPaths) > 0`. Attach offending paths to the `Violation`.
- `T-4` Severity/escalation: an out-of-scope symbol with `structure.Dependents(...).Count > 0` → high-severity; `r-scope` resolves to `block` only when `structure.Available()` and `flow-rules.json` enables it; otherwise `warn`.
- `T-5` `enforce.go` reuse: `r-contract` reprompts once ("declare intended scope before editing", bounded by `max_reprompt_attempts`); SSE `flow_gate_violation` carries offending paths.
- `T-6` Tests — unit: `ScopeDiff` in-scope=∅, out-of-scope set, glob matching, ignore-set exclusion, symbol-level path under a fake provider; rename-only diff resolved to in-scope symbol → **no** `r-scope`. Integration: edit outside `declared_paths` → `r-scope` warn with paths; no-contract turn → `r-contract` once.

## 5. Touched Areas

- files: `apps/local-runner/internal/flowgate/{rules,evaluate}.go`, `internal/changecontract/scope.go` (+ `_test.go`), `settings/flow-rules.json` seed
- modules: `flowgate`, `changecontract`, `structure`
- routes: SSE `flow_gate_violation` (existing)
- tables: `flow_rules` mirror (existing enum)

## 6. Acceptance Check (DoD)

- [x] `r-contract` and `r-scope` exist in `rules.go` and seed to `flow-rules.json` with the actions above. Seeding is automatic — `LoadRules` already runs every loaded rule set through `MergeDefaultRules`, so an existing project's `flow-rules.json` picks up both new rules by ID on next load without a migration step (same mechanism Task-223 used for `r-artifact-output`).
- [x] An in-scope-only turn produces zero scope violations.
- [x] An edit outside `declared_paths` emits `r-scope` (warn) with the exact offending paths in the violation payload.
- [x] With GitNexus present, an out-of-scope path that has dependents is high-severity and blocks when configured; with GitNexus absent, `r-scope` stays `warn`. **Implemented at file level, not symbol level** — see next item.
- [ ] **Not implemented: rename-only/formatter false-drift suppression via symbol resolution (`SD-21 F-2`).** `structure.Provider.Dependents(ctx, target)` takes a file/target path and reports dependents; it has no API to map a diff hunk to a resolved symbol name, and nothing in this codebase (Task-098's provider included) extracts symbols from a diff today. Building that would mean writing new AST/symbol-extraction code — exactly the cost `SD-21 D-2` explicitly avoids by design ("no AST, no code-graph build in v1"). Given that constraint, this task's `ScopeDiff`/`HighSeverity` operate on **paths only**: a rename-only or formatter-only diff on an in-scope *file* is still fine (still matches `declared_paths`), but a rename that also touches an out-of-scope *file* will be flagged the same as any other out-of-scope file touch — there is currently no finer-grained symbol check to suppress that specific false positive. Left open for a follow-up if it proves to matter in practice; `Contract.DeclaredSymbols`/`ScopeDiff`'s `outSymbols` return already exist as the extension point, just unpopulated.
- [x] A turn with no **declared** `Contract` emits `r-contract` exactly once (the inferred-contract case), never blocks. Note: since Task-184's `captureChangeContract` always persists *something* (declared or inferred) on every turn, "no stored Contract at all" cannot occur after Task-184 landed — `r-contract`'s real condition is "the stored Contract for this turn has `confidence=inferred`" (`TurnResult.ContractDeclared == false`), which is the same case Task-184 F-3 describes.
- [x] Doc paths (`requirements/`, `change-audit/`, `*.md`) are excluded from `actual_touched` (via `flowgate.IsDocOrAuditFile`, reused from `r-ca`). **The `E-4` per-project ignore set is not implemented** — it doesn't exist anywhere in this codebase yet (not by `r-tests`/`r-reg`, not by this task); `SS-14 E-4` describes it as a requirement but no prior task built the config surface for it. Out of scope to invent here without a dedicated design; flagged for a future task rather than scope-crept into this one.
- [x] Gate resolves a single highest-severity action (`SD-20` ladder); `go test ./internal/flowgate/... ./internal/changecontract/...` passes.

## 7. Out of Scope

- Canonical Head / signature / spec-drift / code-drift rules (Task-186).
- Decision records and `r-retire` (Task-187).
- Prompt packing and Admin panels (Task-188).

## 8. Completion Notes

- result: **done (2026-07-17) with documented waivers** — file-level scope-drift complete; symbol-level + E-4 not in scope for v1.
- **Landed:** `changecontract/scope.go` (`ScopeDiff`, `HighSeverity`) + tests; `flowgate` `r-contract`/`r-scope` + triggers + enforce; `gate_hook` feeds scope signals into `TurnResult`.
- **DoD (§6):** all file-level boxes checked; rename/format symbol false-drift **waived** (SD-21 D-2 no-AST — no hunk→symbol API); E-4 per-project ignore **waived** (no config surface in codebase; inventing it is out of scope).
- **Verification (prior session):** changecontract + flowgate suites green; runner baseline env flakes only.
- follow-ups (optional, new tasks if needed): symbol extraction for F-2; E-4 ignore config design; Task-186 head-aware drift.
- upstream: none required for close.
