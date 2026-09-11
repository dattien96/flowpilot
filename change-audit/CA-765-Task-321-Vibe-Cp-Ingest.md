# CA-765 — vibe-cp-ingest CP lock and sequential sprint queue

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-321
change_type: feature
summary: Add vibe-cp-ingest pack flow, allow vibe user-start, /vibe-cp path gate, cp_lock blocks sprint, sequential budgeted vibe-sprint
# --->8---

## Change

Task-321 / CP-60 P-6. Operator allowed editing `TestLoadBuiltinPack` `11 → 12`.

- New `flows/vibe-cp-ingest.yaml`: `cp_reader → cp_validator → cp_lock(user.confirm) → task_slicer` (CP-58 splitter bindings). `selectableIn: []`.
- Manifest + `TestLoadBuiltinPack` count 12; inventory pin 12/8.
- `workingmode` vibe user-start: `vibe-ingest` **and** `vibe-cp-ingest`. `/flow` picker still `[vibe-ingest]` only; CP entry is `/vibe-cp`.
- TUI `/vibe-cp <CP-*.md>` rejects non-CP path; arms `vibe-cp-ingest`.
- Runner: ingest start sets `vibeAwaitingLock`; `startResolvedFlow` refuses `vibe-sprint` while locked; `task_slicer` done starts next sprint; budget 8 parks `BlockReason: budget`.

## Tests

- `task321_vibe_cp_ingest_test.go` (pack topology)
- `task321_vibe_cp_test.go` (detect, lock, sequence, budget, replay index)
- `task321_vibe_cp_tui_test.go`
- P-1 `TestFlowAllowed_VibeUserCpIngestForbidden` / HTTP twin now allow (P-6 unpark)

## Provider impact

**Case 1 agnostic.** No `providerKey` on detect/queue/gate.

## Residual

- Manual `/vibe-cp` lock-card click-through and live N× sprint demo not claimed (no P-2 `r-requirement` / P-4 SS lock UX).
- Desktop chrome does not yet have a file picker for CP; TUI `/vibe-cp` is the entry.
- `r-requirement` plain-language card is P-2, not this slice.

## Will not undo

CA-763 / CA-764. CA-755 / CP-58 splitter contract / CP-61.
