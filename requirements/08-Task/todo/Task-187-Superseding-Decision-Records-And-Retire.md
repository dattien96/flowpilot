# Task-187: Superseding Decision Records And Retire

## Metadata

- Document ID: `Task-187`
- Title: `Superseding Decision Records And Retire`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-4), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-3)
- Child Documents: `None`
- Related Documents: [Task-186: Canonical Head And Intent Signature](./Task-186-Canonical-Head-And-Intent-Signature.md), [Task-161: Per-Feature Chat-Summary Timeline](../../08-Task/done/Task-161-Per-Feature-Chat-Summary-Timeline.md)
- Replaces: `None`
- Tags: `decision-records, churn-collapse, retire, canonical-head, changecontract, local-runner`

## AI Quick View

### Summary

- Fold **negative knowledge** into each `CanonicalHead`: which approaches were tried and rejected, and why — so `A → B → C → A` churn reads as "current-A + closed dead-ends", not raw commit noise.
- Sources are deterministic: `chat_summary` rejected approaches + revert-type `changeledger` bugfix entries. Only the summary text is AI-generated (cheap-tier, heuristic fallback).
- Add the end-of-life rule `r-retire` (rename/merge/deprecate) that folds a retiring Head's decisions into its target so knowledge is never lost.

### Current Ask

- Implement `SD-21 P-4`: `Decision` folding, churn-collapse (positive churn not packed), and `r-retire`.

### Key Decisions

- `T-1` Collapse positive churn, promote negative knowledge (`SD-21 D-4`); `A`-then-`A` is byte-equal but not semantically equal.
- `T-2` `r-retire` is `approve` (human-confirmed); merges/renames copy `decisions` into the target Head.

### Constraints

- Reuse `SD-17 §3.2` `chat_summary.ndjson` and `changeledger` bugfix entries; do not add a new AI pass for ordering/linkage.
- Reuse the `SD-16` approval gate for `r-retire`; non-fatal (`AC-9`).

### Open Questions

- How is a retire intent detected — `FEATURE-KEYS.md` diff (key removed/renamed) vs an explicit user action?

### Source Refs

- `SD-21 §5` (`Decision`, retire transitions), `§6` (`r-retire`), `§13.3 T4`, `§13.5 T5`. `CP-43 §4.4`. `SS-14 AC-3` (build on newest, don't undo).

## 1. Goal

Every Head carries an ordered list of rejected alternatives with reasons; positive churn stays in the ledger but is not surfaced; retiring a feature preserves its negative knowledge in the successor.

## 2. Parent Links

- coding plan: `CP-43` P-4
- tech design: `SD-21` §5 (`Decision` + retire), §6 (`r-retire`), D-4
- system spec: `SS-14` AC-3
- specific upstream ids: `P-4`, `r-retire`

## 3. Trigger

The value of a messy history is the rejected approaches and their reasons — the exact thing the raw ordered log buries. This task extracts that so future turns don't re-attempt known dead-ends (`AC-3`).

## 4. Exact Change

- `T-1` `changecontract/decisions.go` — `Decision` struct (`tried, outcome ∈ {adopted,rejected,reverted}, reason, superseded_by, source_doc_id, at`); `FoldDecisions(featureKey) ([]Decision, error)` from (a) `chat_summary.ndjson` rejected approaches for the key, (b) revert-type `changeledger` bugfix entries whose subject/`BUG-` doc reverts a prior feature commit.
- `T-2` `UpdateHead` integration (Task-186): after folding, write `decisions` into the Head; **do not** add positive churn to the packed set — the collapse rule lives here (raw `feature_history` stays available on demand only).
- `T-3` `changecontract/retire.go` — `RetireHead(featureKey, action ∈ {renamed,merged,deprecated}, targets []string) error`: set `status`, `superseded_by`, `retired_at`; for `renamed`/`merged`, copy `decisions` into each target Head; drop the retired Head from default packing but keep the file for provenance.
- `T-4` `flowgate/rules.go` + `evaluate.go` — `r-retire` (`trigger:feature_rename_merge_or_deprecate`, `action:approve`): detect a retire intent (e.g. a `FEATURE-KEYS.md` diff removing/renaming a key, or an explicit control) and require human confirmation of `targets` before `RetireHead` runs; reuse the `SD-16` approval gate.
- `T-5` Tests — `FoldDecisions` extracts rejected/reverted entries with reasons; an `A → B(rejected) → A` sequence yields current-A + a `Decision{tried:B, outcome:rejected}`; packed set excludes B/C churn; `RetireHead` merge copies decisions to the target and sets `superseded_by`; `r-retire` requires approval before mutating.

## 5. Touched Areas

- files: `apps/local-runner/internal/changecontract/{decisions,retire}.go` (+ `_test.go`), `internal/changecontract/update.go` (fold call), `internal/flowgate/{rules,evaluate}.go`, `settings/flow-rules.json`
- modules: `changecontract`, `flowgate`, `changeledger`, chat-summary store
- routes: approval-gate event (SD-16) for `r-retire`
- tables: none (local JSON); `flow_rules` mirror

## 6. Acceptance Check (DoD)

- [ ] `FoldDecisions` returns rejected/reverted approaches with reasons and `source_doc_id`, sourced deterministically from `chat_summary` + revert-type ledger entries.
- [ ] For a feature that went `A → B(rejected) → A`, the Head shows current-A plus a `Decision` for B; the **packed prompt does not replay** B/C positive churn (verified with Task-188 packing, asserted here at the data level).
- [ ] `RetireHead` for `renamed`/`merged` copies `decisions` into each target Head and sets `superseded_by` + `retired_at`; `deprecated` sets `retired_at` and drops from packing while keeping the file.
- [ ] `r-retire` is `approve` and does not mutate any Head until a human confirms the targets (reuses `SD-16` gate).
- [ ] Retired Heads remain retrievable for provenance (not deleted).
- [ ] Only decision *text* is AI-generated; ordering/linkage are deterministic and reproducible.
- [ ] `go test ./internal/changecontract/... ./internal/flowgate/...` passes.

## 7. Out of Scope

- Head creation/signature/drift states (Task-186).
- Head-first prompt packing and Admin panels (Task-188).
- Scope-drift rules (Task-185).

## 8. Completion Notes

- result: planned
- follow-ups: Task-188 renders the folded decisions in the packed Head block and the Admin panel.
- upstream docs updated: none
