# Task-098: GitNexus Structure Provider

## Metadata

- Document ID: `Task-098`
- Title: `GitNexus Structure Provider`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-102: Tooling Check And Capability Profile](./Task-102-Tooling-Check-And-Capability-Profile.md), [Task-099: Post-Step Flow Gate](./Task-099-Post-Step-Flow-Gate.md)
- Replaces: `None`
- Tags: `structure, gitnexus, plane-b, local-runner`

## AI Quick View

### Summary

- Wire GitNexus as a pluggable structure provider over the bound repo (Plane B): reverse dependents, summarized, with a completeness flag.
- Degrade cleanly to file-level neighbors when GitNexus is unavailable. No bespoke indexer.
- Powers the `code.dependents` slot and the `removed_referenced_code` gate rule (AC-4).

### Current Ask

- Implement `internal/structure/` (`provider`, `gitnexus`, `fallback`) per CP-35 §4.3.

### Key Decisions

- `T-1` `Provider` interface with `Available()` + `Dependents()` returning a summary (`Count`, `Nearest`, `Flows`, `Complete`).
- `T-2` Never raw-dump dependents; mark `Complete=false` where dynamic dispatch is suspected.

### Constraints

- Optional capability; absence must not fail the engine. GitNexus install/health is Task-102. Symbol-level scope-drift stays deferred (`SD-17 D-11`).

### Open Questions

- GitNexus run trigger + staleness suppression (`SD-17 Q-3`); exact query interface (MCP `gitnexus_impact` vs CLI JSON).

### Source Refs

- `CP-35 §4.3` (P-3); `SD-17 D-5`, §3.4; `SS-14 AC-5`, `AC-4`.

## 1. Goal

A structure provider that returns a summarized blast radius for an edited symbol/file when GitNexus is present, and a file-level fallback when it is not.

## 2. Parent Links

- coding plan: `CP-35` P-3
- tech design: `SD-17` `D-5`, §3.4
- system spec: `SS-14` AC-5, AC-4
- specific upstream ids: `P-3`, `D-5`, `AC-5`

## 3. Trigger

Whole-project context (who depends on what) is a structural question the commit ledger cannot answer; GitNexus already indexes any repo.

## 4. Exact Change

- `T-1` `provider.go` — `Provider` interface + `DependentsSummary` type.
- `T-2` `gitnexus.go` — `Available()` via tooling registry; `analyze` on bind (gated, background); `Dependents()` via `gitnexus_impact`/CLI JSON; summarize + `Complete` flag.
- `T-3` `fallback.go` — file-level neighbors (import scan + ledger co-change).
- `T-4` register `code.dependents` slot (priority 2).
- `T-5` unit tests: absent → fallback; summary shape; `Complete` flag.

## 5. Touched Areas

- files: `apps/local-runner/internal/structure/*`, `internal/contextresolver/` (slot)
- modules: `structure`, `contextresolver`
- routes: none
- tables: `step_context_slots` (enum)

## 6. Acceptance Check

- CP-35 P-3 DoD: GitNexus present → summarized dependents with `Complete`; absent → file-level fallback, flagged low-confidence.
- `go test ./internal/structure/...` passes.

## 7. Out of Scope

- Building our own symbol indexer (deferred), installing GitNexus (Task-102), scope-drift enforcement.

## 8. Completion Notes

- result: planned
- follow-ups: enables `removed_referenced_code` rule in Task-099
- upstream docs updated: none
