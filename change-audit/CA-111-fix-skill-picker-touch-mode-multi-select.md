# CA-111: Fix Skill Picker Touch Mode Multi-Select

## Scope

Single-file change: `apps/desktop-flowpilot/src/components/ChatInput.tsx`

The skill picker has two entry modes:
- **Touch/button mode** — user clicks the Skill block in the controller strip to open the picker. Intended to support multi-select (checkbox UI, "pick one or more" label).
- **Command/slash mode** — user types `/` in the textarea; picker auto-opens, closes after one pick and replaces the `/query` fragment in the text.

`pickSkill()` was calling `setSkillPickerOpen(false)` unconditionally for both modes. This made multi-select impossible in touch mode: the picker closed immediately after each selection.

## Completed

- Moved `setSkillPickerOpen(false)`, `setPickerHighlightIndex(-1)`, and `setSlashDismissedIndex(null)` inside the `if (slashFragment !== null)` branch in `pickSkill()`.
- Touch/button mode (`slashFragment === null`) no longer auto-closes the picker on each skill pick. The user can select any number of skills and dismiss the picker manually via the `×` button or Escape key.
- Command/slash mode retains its existing single-pick-then-close behavior unchanged.
- No store, runner, backend, or IPC changes.

## Verification

- Manual verification required — Electron app; browser preview cannot load skills over IPC.
- Touch mode path: open picker via Skill block → click two skills → verify picker stays open and both checkboxes show `☑` → close via `×`.
- Command mode path: type `/skillname` in textarea → pick from picker → verify picker closes and fragment replaced in text.

## Residual Notes

- No automated UI tests cover this component; regression relies on manual testing.
- Related doc: `requirements/09-BugFix/done/BUG-096-Desktop-Chat-Skill-Picker-Touch-Mode-Does-Not-Allow-Multi-Select.md`
