# CA-823 — next vibe-sprint abandons the previous frozen contract

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-368
change_type: bugfix
summary: Abandon active frozen contracts when starting the next vibe-sprint so Task-911/912 can freeze new paths instead of reusing sprint-1 game.go
# --->8---

## Why

Live run-654339 showed `task 3/3 · done` with only Task-910 coded.
`runContractFreezeNode` reuses an active `(runID, coder)` contract as
crash-recovery. Sequential sprints share that key, so Continue never
unfroze `snake/game.go`.

## Change

- `vibe_sprint_freeze.go`: `abandonActiveFrozenContractsForRun`
- Called from `maybeStartNextVibeSprint` and `startTakenVibeSprint`
  (operator Continue) before `startResolvedFlow`
- `contract-planner.md` / implement prompt: declare **this** Task's files
- Recovery reuse inside `runContractFreezeNode` is unchanged

## Tests

`bug368_vibe_sprint_new_freeze_test.go` (new). Old
`TestRecoveredFlowReusesPersistedFrozenContract` and
`TestDuplicateFreezeDeliveryReturnsExistingContract` green, untouched.

## Providers

Agnostic Case 1.

## Will not undo

CP-55 freeze reuse for duplicate delivery. CP-60 `vibe-sprint`
`selectableIn: []`. BUG-367 task x/y + DoD stamps.
