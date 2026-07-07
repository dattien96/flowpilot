# Task-185: Scope-Drift Detection

## Metadata

- Document ID: `Task-185`
- Title: `Scope-Drift Detection`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-2), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3, AC-7)
- Child Documents: `None`
- Related Documents: [Task-184: Change Contract Capture](./Task-184-Change-Contract-Capture.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [Task-098: GitNexus Structure Provider](../../08-Task/done/Task-098-GitNexus-Structure-Provider.md)
- Replaces: `None`
- Tags: `scope-drift, flowgate, gitnexus, structure, local-runner`

## AI Quick View

### Summary

- Add flow-gate rules `r-contract` (no declaration) and `r-scope` (edit outside the declared contract), evaluated from the gate's existing `TurnResult.GitDiff`.
- Drift = a deterministic set difference `actual_touched \ declared_scope`; file-level always, symbol-level only when `structure.Available()` (GitNexus).
- Warn-first: `r-scope` blocks only when structure is available and configured; deliberately avoids the AST-hashing that got `SD-17 D-11` deferred.

### Current Ask

- Implement `SD-21 P-2`: two rules in `flowgate`, the scope-diff helper, and their evaluation + enforcement wiring.

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

- [ ] `r-contract` and `r-scope` exist in `rules.go` and seed to `flow-rules.json` with the actions above.
- [ ] An in-scope-only turn produces zero scope violations.
- [ ] An edit outside `declared_paths` emits `r-scope` (warn) with the exact offending paths in the violation payload.
- [ ] With GitNexus present, an out-of-scope symbol that has dependents is high-severity and blocks when configured; with GitNexus absent, `r-scope` stays `warn`.
- [ ] A **rename-only / formatter** diff does not trip `r-scope` when the structure provider resolves it to an in-scope symbol (`SD-21 F-2`).
- [ ] A turn with no stored `Contract` emits `r-contract` exactly once, then falls back to an inferred contract (Task-184).
- [ ] Doc paths (`requirements/`, `change-audit/`, `*.md`) and the `E-4` ignore set are excluded from `actual_touched`.
- [ ] Gate resolves a single highest-severity action (`SD-20` ladder); `go test ./internal/flowgate/... ./internal/changecontract/...` passes.

## 7. Out of Scope

- Canonical Head / signature / spec-drift / code-drift rules (Task-186).
- Decision records and `r-retire` (Task-187).
- Prompt packing and Admin panels (Task-188).

## 8. Completion Notes

- result: planned
- follow-ups: Task-186 adds the drift rules that also consult the Head; this task only compares against the per-turn Contract.
- upstream docs updated: none
