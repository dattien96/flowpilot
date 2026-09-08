# CA-767 — CP-60 P-6/P-7 remaining: auto-detect, lock card, Task plan, drift, tdd guard

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: Auto-detect CP vs raw vibe entry, park SS/CP Preview & Lock, parse Task-*.md, inject SS signature drift, refuse coder without tdd artifact
# --->8---

## Change

Closes remaining Task-321 / Task-323 gaps after CA-764/CA-765/CA-766.

- P-6: `DetectVibeEntry` on TUI `/vibe` and Desktop first turn; TUI sends `sourceDocId`; Desktop path field in vibe mode.
- P-6: `user.confirm` `ss_lock`/`cp_lock` parks `BlockReason: vibe_lock` with draft in GateReason; Continue/Lock write-back + `RejectNonCP` then advances.
- P-6: `task_slicer` reads `requirements/08-Task/todo/Task-*.md` (sprint-0 only if empty). `audit → done` chains the next `vibe-sprint`.
- P-7: green vibe turns compute `RequirementDrift` from locked SS `AC-*` vs written tests. `tdd → coder` refuses spawn without a test file.

## Tests

- `workingmode/cp_detect_test.go`
- `flowgate/requirement_signature_test.go`
- `runner/task321_p6_remaining_test.go`
- `tui/app/task321_vibe_autodetect_tui_test.go`
- `desktop-flowpilot/src/state/detectVibeEntry.test.ts`

## Provider impact

**Case 1 agnostic.** Detect/lock/queue/drift/tdd guard do not branch on `providerKey`.

## Residual

- Live Desktop/TUI lock-card click-through and N× sprint demo still manual.
- `r-requirement` card uses GateReason/detail, not a dedicated non-tech renderer.
- Session persist of `vibeTaskPlan` / `vibeLockedSS` not added (restart mid-lock relies on loop BlockReason).

## Will not undo

CA-763 / CA-764 / CA-765 / CA-766. TestDefaultRules count. CP-58 splitter.
