# BUG-368 — Next vibe-sprint reuses sprint-1 frozen contract

## Metadata

- Document ID: `BUG-368`
- Title: `Sequential vibe-sprints on one run reuse the previous frozen contract so Task-911/912 cannot write new files`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-10`
- Last Updated: `2026-09-10`
- Parent Documents: [CP-60](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Child Documents: `None`
- Related Documents: [BUG-367](./BUG-367-Vibe-Task-Status-DoD-And-Progress-Chip.md), [CA-817](../../../change-audit/CA-817-Vibe-Sprint-Boundary-Continue-Gate.md)
- Replaces: `None`
- Tags: `vibe-mode, change-contract, freeze, live-bed`

## AI Quick View

### Summary

- Live run-654339: three vibe-sprint slots ran (`task 3/3 · done`) but `snake/` only contains Task-910 core model. Frozen store for that run has a single intent (core model) and `declared_paths: [snake/game.go, snake/game_test.go]` (v1 + amend v2). No tick/playable freeze ever existed.
- Cause: `runContractFreezeNode` reuses any active `(runID, coder)` contract as duplicate-delivery recovery. Sequential sprints share the parent run and step ids `tdd`/`coder`, so sprint 2/3 kept sprint 1's scope.
- Fix: abandon active frozen contracts for the run when starting the next vibe-sprint (`maybeStartNextVibeSprint` and `startTakenVibeSprint`). Recovery re-delivery of the same freeze still reuses (old test untouched). Planner prompt: declare **this** Task's files.

### Current Ask

- Continue / next sprint must freeze the new Task's paths, not reuse sprint-1's `game.go` list.

### Key Decisions

- `V-1` Do not change the recovery reuse rule inside `runContractFreezeNode` (different draft on the same freeze still reuses). Split by **when** we abandon: only at next-sprint start.
- `V-2` `vibe-sprint` stays `selectableIn: []`. Operator path is vibe-ingest + Continue at the sprint-boundary card, not `/flow vibe-sprint`.
- `V-3` From-scratch on a clean workspace after this fix: 910 then Continue → 911 then Continue → 912, each with its own freeze.

### Constraints

- `feature_key: vibe-mode` (dominant); freeze store is `change-contract`.
- R1: additive tests only; `TestRecoveredFlowReusesPersistedFrozenContract` / `TestDuplicateFreezeDeliveryReturnsExistingContract` stay green.
- R2: agnostic (no providerKey).

### Open Questions

- Re-running vibe-ingest on the dirty gate-sandbox may duplicate CP/Task files; prefer a clean module or Continue remaining Tasks after rebuild.

### Source Refs

- run-654339 frozen_contracts.ndjson (coder/tdd v1+v2, core-model only)
- `runContractFreezeNode` reuse block (`flow_validate_audit_dispatch.go`)
- `vibe-sprint.yaml` `selectableIn: []`

## 1. Issue Summary

After Continue, sprint 2's freeze was a no-op reuse of sprint 1. The coder could not write Task-911/912 files without scope drift. The loop still counted 3/3 and settled done.

## 2. Parent Links

- impacted coding plan: CP-60 sprint loop, CP-55 P-3/P-4 freeze/amend
- impacted tech design: SD-24, SD-21
- impacted system spec: SS-18

## 3. Environment and Reproduction

- environment: TUI vibe-ingest on gate-sandbox, grok-4.5, run-654339
- reproduction steps: lock SS/CP → 3 Tasks sliced → sprint 1 codes `snake/game.go` → Continue → sprint 2 freeze reuses core-model contract
- frequency: every multi-task vibe run on one parent run

## 4. Expected vs Actual

- expected: sprint 2 freeze declares Task-911 files; coder may write them
- actual: GetFrozenForStep still returns sprint-1 `game.go` / `game_test.go`

## 5. Impact

- users affected: every vibe-ingest with >1 Task
- workflows affected: vibe-ingest → vibe-sprint chain
- severity: high (later Tasks cannot land)

## 6. Root Cause

- hypothesis: freeze reuse is keyed only by `(runID, writerStepID)`
- confirmed cause: reuse block treats any active contract as duplicate delivery; vibe-sprints never abandon it
- evidence: frozen store dump + `TestRecoveredFlowReusesPersistedFrozenContract` documents the recovery rule we must keep

## 7. Fix Strategy

- `F-1` `abandonActiveFrozenContractsForRun` before `startResolvedFlow` on next sprint
- `F-2` Call from `maybeStartNextVibeSprint` and `startTakenVibeSprint` (Continue)
- `F-3` Planner: read the current Task file; do not copy leftover declared_paths

## 8. Validation

- `V-1` New `TestBUG368_*`: abandon then new freeze gets new paths; without abandon, different draft still reuses; maybeStart / Continue abandon
- `V-2` Old recovery + duplicate-delivery tests green and untouched

## 9. Regression Guard

- tests: `runner/bug368_vibe_sprint_new_freeze_test.go`
- alerts: none
- audit checks: CA-823

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: CP-55 freeze reuse for recovery; CP-60 `vibe-sprint` hidden from `/flow`
