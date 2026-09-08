# CA-784 — vibe on/off drops armed flow back to chat

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: bugfix
summary: /vibe on and /vibe off (and Desktop toggle) leave the current flow and return to chat mode
# --->8---

## Why

Toggling vibe while armed on task-harness kept Flow chrome. Dev harness is illegal in vibe; vibe ingest is illegal in dev. Mode switch must drop the arm.

## Change

- TUI `setWorkingMode`: ModeChat + empty launch
- Desktop `setWorkingMode`: chatStartMode=normal, clear source/flowRef
- `/vibe <requirement>` still re-arms ingest after the drop

## Tests

New `ca784_vibe_toggle_drops_flow_test.go`, `ca784_working_mode_resets_chat.test.ts`. Old Task-326 tests untouched.

## Will not undo

CA-781/782 picker. CA-783 slicer→sprint.
