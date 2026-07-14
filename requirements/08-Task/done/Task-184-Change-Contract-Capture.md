# Task-184: Change Contract Capture

## Metadata

- Document ID: `Task-184`
- Title: `Change Contract Capture`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-13` (implemented — `internal/changecontract/` built, wired into `gate_hook.go`, `context-discipline` skill bumped; see §6/§8)
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-1), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (US-3)
- Child Documents: `None`
- Related Documents: [Task-185: Scope-Drift Detection](./Task-185-Scope-Drift-Detection.md), [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/done/CP-35-Context-And-Regression-Engine-Rollout.md) (P-2 resolver, P-6 skill pack)
- Replaces: `None`
- Tags: `change-contract, changecontract, contextresolver, skill-pack, local-runner`

## AI Quick View

### Summary

- Build `internal/changecontract/` contract capture: before a code turn, record what the AI intends to change (`feature_key`, `intent`, `declared_paths`).
- Add a pre-turn resolver slot `contract.declare` (after CP-35 `feature.resolve`) and an inferred-from-diff fallback when the AI does not declare.
- Store one `Contract` per `(run_id, step_id)` locally; never sync.

### Current Ask

- Implement `SD-21 P-1`: the `Contract` type + NDJSON store, the declaration parser, the `contract.declare` slot, and the inferred fallback.

### Key Decisions

- `T-1` Contract is **per-turn**, not per-feature; local-only; last-wins by `(run_id, step_id)`.
- `T-2` No declaration → `confidence="inferred"` synthesized from the first diff; confirm only under `gate_mode=enforce` (`SD-21 Q-1`/D-7).

### Constraints

- Reuse `feature.resolve` (CP-35 Task-097) for `feature_key`; do not re-implement feature resolution.
- Non-fatal: a capture failure degrades to `inferred`/empty and never blocks the turn (`AC-9`).
- Mirror `local_file_session_store.go` NDJSON + mutex pattern.

### Open Questions

- Explicit declaration turn vs inferred-then-confirm as the default UX (`SD-21 Q-1`).

### Source Refs

- `SD-21 §4` (new module), `§5` (`Contract`), `§7` (steps 1), `§13.1`/`§13.3 T1`. `CP-43 §4.1`. `SS-14 US-3`.

## 1. Goal

Every code-mutating step begins with a stored `Contract` describing the intended feature, goal, and file scope, so Task-185 can flag anything edited outside it.

## 2. Parent Links

- coding plan: `CP-43` P-1
- tech design: `SD-21` §4, §5 (`Contract`), §7 step 1, D-7
- system spec: `SS-14` US-3
- specific upstream ids: `P-1`, `US-3`

## 3. Trigger

`SS-14 US-3` requires the AI to declare intended changes. CP-35 handles context-in and post-turn output-forcing, but there is no record of *intended scope* to compare the actual diff against. This task captures that record.

## 4. Exact Change

- `T-1` `internal/changecontract/contract.go` — `Contract` struct (`run_id, step_id, feature_key, intent, declared_paths []string, declared_symbols []string, declared_at, confidence`); `Store` at `.flowpilot/contracts/contracts.ndjson`, mutex-guarded, last-wins by `(run_id, step_id)`; `Save(Contract) error`, `Get(runID, stepID) (Contract, bool)`.
- `T-2` `internal/changecontract/parse.go` — `ParseDeclaration(text string) (Contract, bool)`: extract a fenced/labeled declaration block (`feature:`, `intent:`, `files:`) the skill emits; tolerant of missing lines; returns `ok=false` when no block is present.
- `T-3` `internal/contextresolver` slot `contract.declare` (priority 1, pre-turn, runs after `feature.resolve`): reads the resolved `feature_key`, parses the turn's declaration into a `Contract` (`confidence="declared"`), and persists it via `Store.Save`.
- `T-4` `internal/changecontract/infer.go` — `InferFromDiff(featureKey string, diff []ChangedFile) Contract`: when no declaration exists, set `declared_paths` = distinct top-level dirs of touched non-doc files, `confidence="inferred"`; used post-diff by Task-185 to still evaluate scope. Confirm the inferred contract to the user only when `gate_mode=enforce`.
- `T-5` Skill: extend the `context-discipline` skill (CP-35 P-6 pack) with a "declare scope before editing" clause that instructs the AI to emit the declaration block; bump the pack version.
- `T-6` Unit tests: `ParseDeclaration` (full block, partial, absent); `Store` round-trip + last-wins; `InferFromDiff` top-level bucketing + doc-file exclusion.

## 5. Touched Areas

- files: `apps/local-runner/internal/changecontract/{contract,parse,infer}.go` (+ `_test.go`), `internal/contextresolver/` (slot registration), `internal/skillpack/flow-pack/context-discipline/SKILL.md`
- modules: `changecontract`, `contextresolver`, `skillpack`
- routes: none
- tables: none (local NDJSON); new `step_context_slots.resolver` enum value `contract.declare`

## 6. Acceptance Check (DoD)

- [x] `Contract` struct persists to `.flowpilot/contracts/contracts.ndjson`, one line per `(run_id, step_id)`, last-wins on re-save.
- [x] Declared-scope capture runs on every code-mutating turn and yields `confidence="declared"` with parsed `feature_key`/`intent`/`declared_paths`. **Implementation note:** the doc's original "`contract.declare` resolver slot at priority 1, pre-turn, after `feature.resolve`" wording predates CP-44's `ContextSourceRegistry` (which didn't exist when this Task was authored) and doesn't map onto it — `ContextSource.Fetch` is for *fetching content to inject*, not *parsing the AI's own declaration back out of its message*. Implemented instead as `captureChangeContract` (`gate_hook.go`), called from the existing post-`finishTurn` gate hook (`runFlowGate`) right after the diff/`suggestedFeatureKeys` are observed — the only point where both the AI's `FinalMessage` (to parse) and the `GitDiff` (for the inferred fallback) are already available. Functionally equivalent to the spec's intent; timing is post-turn instead of pre-turn since a declaration can only be read back from a message that has already been generated.
- [x] A turn with no declaration produces an `inferred` contract from the first diff and does **not** block.
- [x] `ParseDeclaration` returns `ok=false` (not an error) when no block is present; malformed/partial blocks degrade gracefully (missing lines just leave those fields empty).
- [x] `InferFromDiff` excludes `requirements/`, `change-audit/`, `*.md` (via `flowgate.IsDocOrAuditFile`) and buckets by top-level dir.
- [x] `context-discipline` skill contains the declare-scope clause (rule 6); pack version bumped **5→6 globally** (`skillpack.PackVersion` + all 17 skill files' own `version:` field, not just this one — `fileMatchesVersion` compares every file against the single pack-wide constant); re-bind re-syncs it (CP-35 P-6).
- [x] `contracts.ndjson` is **not** added to the `contextsync` shared set — confirmed by inspection: `contextsync/local.go` uses an explicit allowlist (`ledger/`, `catalog/`, `settings/`, `guard/`, `structure/` subdirs + two named ledger files); `contracts/` isn't in it, so it's local-only by construction, no extra exclusion code needed.
- [x] `go build ./...` and `go test ./internal/changecontract/...` pass; no regression in `contextresolver` tests. **Implementation note:** there is no `internal/contextresolver` package in this codebase (see note above) — the equivalent regression check is `go test ./internal/runner/...` (which covers `context_source_registry_test.go` and the gate-hook path this change touches).

## 7. Out of Scope

- Scope-diff evaluation and the `r-contract`/`r-scope` rules (Task-185).
- Canonical Head, signature, drift states (Task-186).
- Admin visualization (Task-188).

## 8. Completion Notes

- result: **done** (2026-07-13). New files: `apps/local-runner/internal/changecontract/{contract,parse,infer}.go` + `_test.go` (15 tests). Modified: `apps/local-runner/internal/runner/gate_hook.go` (new `captureChangeContract` call + function, wired into `runFlowGate`), `apps/local-runner/internal/skillpack/flow-pack/common/context-discipline/SKILL.md` (rule 6 + version 6), `apps/local-runner/internal/skillpack/install.go` (`PackVersion = 6`), all 16 other skill files (`version: 5` → `6`, lockstep pack version).
- verification: `go build ./...` clean; `go vet ./internal/changecontract/... ./internal/runner/... ./internal/skillpack/...` clean; `go test ./internal/changecontract/...` 15 passed; `go test ./internal/skillpack/...` 13 passed; `go test ./internal/runner/... -count=1` 1386 passed / 16 failed (all pre-existing environment-dependent flakes — Codex CLI resume, account-home, skills-merge, auth-workspace — identical to the baseline established earlier this session; zero new failures).
- follow-ups: Task-185 consumes the stored `Contract` (via `Store.Get(runID, stepID)`) to compute scope drift (`r-contract`/`r-scope`).
- upstream docs updated: none (SD-21/CP-43 already reflect this design; no design change was needed to implement it — only the "contract.declare slot" wording turned out to be stale relative to CP-44, noted in §6 rather than requiring an SD-21 edit).
