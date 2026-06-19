# BUG-096: Desktop Chat Skill Picker Touch Mode Does Not Allow Multi-Select

## Metadata

- Document ID: `BUG-096`
- Title: `Desktop Chat Skill Picker Touch Mode Does Not Allow Multi-Select`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [Task-079: Chat Skill Picker Keyboard Command Flow](../../08-Task/done/Task-079-Chat-Skill-Picker-Keyboard-Command-Flow.md), [Task-053: Desktop Chat Turn Skills Summary Chip](../../08-Task/done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md)
- Child Documents: `none`
- Related Documents: [CA-111: Fix Skill Picker Touch Mode Multi-Select](../../../change-audit/CA-111-fix-skill-picker-touch-mode-multi-select.md)
- Replaces: `none`
- Tags: `desktop, ui, chat, skills, skill-picker, touch-mode, multi-select, regression`

## AI Quick View

### Summary

- The skill picker has two distinct entry modes: **touch/button mode** (user clicks the Skill block to open the picker) and **command mode** (user types `/` in the textarea to trigger the picker).
- Touch mode is intended to allow **multi-select** — the picker should stay open so the user can tick multiple skills before dismissing it manually.
- Command mode is intended to allow **single-select per slash token** — the picker closes after one pick and the `/query` fragment is replaced in the text.
- A single unconditional `setSkillPickerOpen(false)` call in `pickSkill()` closed the picker after every selection in **both** modes, making multi-select impossible in touch mode.

### Current Ask

- Done. `F-1` implemented in `ChatInput.tsx`.

### Key Decisions

- `V-1` Mode is distinguished by `slashFragment !== null` (command mode) vs. `slashFragment === null` (touch/button mode); no new state needed.
- `V-2` Picker close logic (`setSkillPickerOpen`, `setPickerHighlightIndex`, `setSlashDismissedIndex`) moved inside the `slashFragment !== null` branch so touch mode never auto-closes.
- `V-3` Touch mode dismissal is handled by the existing `×` close button and Escape key — no new UX needed.

### Constraints

- Fix is scoped to `pickSkill()` in `ChatInput.tsx` only; no store, runner, or backend changes.
- The picker header already reads "Skills · pick one or more" — label is consistent with the fixed behavior.

### Open Questions

- None.

### Source Refs

- `Task-079`, `Task-053`
- `apps/desktop-flowpilot/src/components/ChatInput.tsx` lines 400–426 (pre-fix)

## 1. Issue Summary

The skill picker in Desktop Chat supports two open modes: button-click (touch mode) and slash-command (command mode). In touch mode the user taps the Skill block to open the picker and should be able to select multiple skills before closing it manually. In command mode the picker opens on a `/` fragment and should close after a single pick (the fragment is replaced in the prompt text).

After `Task-079` introduced the slash-command flow, the `pickSkill()` function was left with `setSkillPickerOpen(false)` called unconditionally at the end of every pick, regardless of which mode opened the picker. This caused the picker to close after every single selection in touch mode, making multi-select effectively impossible without reopening the picker for each skill.

## 2. Parent Links

- impacted coding plan: [Task-079: Chat Skill Picker Keyboard Command Flow](../../08-Task/done/Task-079-Chat-Skill-Picker-Keyboard-Command-Flow.md)
- impacted tech design: n/a (UI-only change)
- impacted system spec: n/a

## 3. Environment and Reproduction

- environment: Desktop FlowPilot (Electron + React), any OS
- reproduction steps:
  1. Open Desktop FlowPilot and select a provider.
  2. In Chat mode, click the **Skill** block in the controller strip to open the picker (touch/button mode).
  3. Click any skill to select it (checkbox turns `☑`).
  4. Observe that the picker immediately closes — only one skill is selected.
  5. Reopen the picker; clicking a second skill closes the picker again.
- frequency: 100% reproducible

## 4. Expected vs Actual

- expected: In touch/button mode the picker stays open after each selection; the user selects multiple skills and dismisses the picker manually (via `×` or Escape). In command/slash mode the picker closes after one pick as before.
- actual: Picker closes immediately after the first pick in **both** modes, preventing multi-select in touch mode.

## 5. Impact

- users affected: All desktop chat users who rely on multi-skill selection via the Skill block button.
- workflows affected: Any chat turn that should carry more than one skill context.
- severity: Medium — the feature is partially broken (multi-select state and UI exist, but UX prevents using them).

## 6. Root Cause

- hypothesis: Task-079 added slash-command single-select behavior but left the picker-close call unconditional.
- confirmed cause: In `pickSkill()` (`ChatInput.tsx` ~line 423), `setSkillPickerOpen(false)` was called after the `if (slashFragment !== null)` block, so it ran unconditionally for both touch mode and command mode.
- evidence: Code inspection of `ChatInput.tsx` lines 402–426 confirms `setSkillPickerOpen(false)` and related resets were outside the `if (slashFragment !== null)` guard.

## 7. Fix Strategy

- `F-1` Move `setSkillPickerOpen(false)`, `setPickerHighlightIndex(-1)`, and `setSlashDismissedIndex(null)` inside the `if (slashFragment !== null)` branch in `pickSkill()`. Touch/button mode (`slashFragment === null`) no longer auto-closes the picker; command mode retains its existing close-on-pick behavior.

## 8. Validation

- `V-1` Manual test — touch/button mode: open picker via Skill block, click two or more skills, verify picker remains open and both checkboxes show `☑`, then close via `×`.
- `V-2` Manual test — command mode: type `/cook` in textarea, select a skill from picker, verify picker closes and `/cook` is replaced in prompt text; behavior unchanged.
- `V-3` Existing Escape and `×` close paths still dismiss the picker in both modes.
- Note: Verification could not be run automatically — this is an Electron app; browser preview cannot exercise skill loading over IPC. Manual verification required.

## 9. Regression Guard

- tests: No automated UI tests exist for this component; manual regression on both picker modes after each ChatInput change.
- alerts: n/a
- audit checks: Confirm `pickSkill()` close logic remains inside `slashFragment !== null` block.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — Task-079 constraint "Skills selected via the button-click Skill box retain old silent-select behavior" referred to text injection, not multi-select. The multi-select intent was always stated in the picker header ("pick one or more"); this fix restores it.
- notes left unchanged on purpose: Task-079 documents the slash-command flow correctly; no edits needed there.
