# CA-783 — sprint_slicer hub done starts vibe-sprint

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: sprint_slicer/task_slicer --done--> done claims the hub turn and starts vibe-sprint instead of settling ingest
# --->8---

## Why

Live run-213752: after Lock, sprint_slicer completed and vibe-ingest settled `done`. No vibe-sprint. `advanceHubDoneThroughEdge` returns false on terminal `done`, so `onVibeCpNodeDone` never ran.

## Change

- Terminal done from `sprint_slicer` / `task_slicer` calls `onVibeCpNodeDone` and returns handled advancing (skip ingest settle)
- Branch V plan: SS-*.md when no Task-*.md in todo

## Tests

`ca783_slicer_starts_sprint_test.go` (Claude/Codex/Grok). Old `TestVibeIngest_SprintSlicerSeedsPlan` untouched.

## Will not undo

CA-777 ss_lock park. CA-780 slicer dispatch after lock.
