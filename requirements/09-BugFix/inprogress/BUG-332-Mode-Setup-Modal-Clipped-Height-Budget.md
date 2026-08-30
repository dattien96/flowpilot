# BUG-332: /mode-setup modal clipped to header+tabs — fields/buttons pushed past terminal bottom

## Metadata

- Document ID: `BUG-332`
- Title: `/mode-setup modal clipped to header+tabs — fields/buttons pushed past terminal bottom`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Feature Keys: `cli-tui`
- Parent Documents: [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md) (section H re-test), [CA-685 posture NON mode]
- Child Documents: `none`
- Related Documents: e238f196 (modal introduction), BUG-331 (same re-test session)
- Replaces: `none`
- Tags: `tui, layout, modal, severity-medium`

## AI Quick View

### Summary

- Operator screenshot: the "Configure postures" modal showed only the title + SCAN/PLAN/CODE/NON tab row at the bottom edge of the terminal; the Model/Reasoning/YOLO fields, provider hint, Save/Cancel and the composer were gone for ALL tabs.
- `renderChatPane` appends the modal block after the transcript, but `tuiChrome()` never subtracted the modal's height from `messagesHeight` (the `modalH` term never existed in history — latent since e238f196 introduced the modal; suggestions previously occupied a similar slot which masked some heights). Frame rows exceeded the terminal height `h` and the renderer clipped the bottom: modal bottom half + composer vanished.

### Fix

1. `tuiChrome` (mouse.go): new `modalBlock`/`modalH` fields; while `modeSetupModalOpen` the block is rendered ONCE here and `modalH` is subtracted in the `messagesHeight` budget (transcript shrinks while the modal is open — exact budget, no overflow at any terminal height ≥ the floor).
2. Mouse/cursor Y chain stays truthful: with the modal open the suggestions block is suppressed, so `attachPanelY` steps over `modalH` instead of `suggLines`.
3. `renderChatPane` (app.go): renders the cached `c.modalBlock` (the exact string `modalH` was measured from) instead of re-rendering.

## Validation

- `bug332_mode_setup_modal_height_test.go` (3, additive): modal height is budgeted (`messagesHeight` subtracts `modalH`); the rendered chat pane fits the terminal height with the modal open AND contains the Model field row, the bottom border, and the title; closed-modal layout equals the legacy formula (no behavior change when the modal is shut).
- R1: full `tui/app` package with the fix shows exactly the 7 documented pre-existing failures; stashing the fix and rerunning those 7 reproduces the identical failures → zero new regressions.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-57
change_type: bugfix
summary: Budget the /mode-setup modal height in tuiChrome so the posture modal fields and composer are never clipped off-screen
# --->8---
