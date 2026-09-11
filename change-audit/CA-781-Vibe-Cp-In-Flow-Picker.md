# CA-781 — vibe `/flow` lists vibe-cp-ingest; Tab/Enter opens @ file picker

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-326
change_type: feature
summary: Operator override of T-3 — vibe /flow lists vibe-cp-ingest; Tab or Enter forces the @ workspace file picker; arm only with a CP-*.md path
# --->8---

## Why

Operator: vibe-cp-ingest is a flow and must appear in `/flow`. Selecting it (Tab or Enter) must open the same file picker as `@`, because the run needs a CP path.

## Change

- `FlowPickerOptions(vibe)` = `[vibe-ingest, vibe-cp-ingest]` (Desktop `flowPickerOptions` matched)
- TUI synthesizes missing picker ids so cache without `vibe-cp-ingest` still lists it
- `suggestionAcceptValue` / Enter `expandsPicker` for that id → `/flow vibe-cp-ingest @`
- `/flow vibe-cp-ingest` with no path reopens the picker; `RejectNonCP` on the path; arm `SourceDocID`

## Tests

`ca781_vibe_cp_flow_picker_test.go`. Task-326 picker tests updated to the new list (operator AC). Dev still omits vibe-*.

## Will not undo

CA-774/775 start vibe-ingest. CA-780 slicer after SS lock. `/vibe-cp` slash still works.
