# BUG-067: Desktop Chat Turn Skills Chip Hidden After Slash Pick

## Metadata

- Document ID: `BUG-067`
- Title: `Desktop Chat Turn Skills Chip Hidden After Slash Pick`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-053: Desktop Chat Turn Skills Summary Chip](../../08-Task/done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md), [Task-048: Desktop Chat Skills Workflow Composer Alignment](../../08-Task/done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md), [Task-051: Desktop Chat Empty Project Guard And Control Header Cleanup](../../08-Task/done/Task-051-Desktop-Chat-Empty-Project-Guard-And-Control-Header-Cleanup.md)
- Child Documents: `none`
- Related Documents: [BUG-062: Desktop Chat Skill Picker Clears Prompt And Ignores Provider Folders](./BUG-062-Desktop-Chat-Skill-Picker-Clears-Prompt-And-Ignores-Provider-Folders.md), [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [CA-070: Desktop Chat Turn Skills Summary Chip](../../../change-audit/CA-070-desktop-chat-turn-skills-summary-chip.md), [CA-077: Fix Desktop Chat Turn Skills Summary Visibility](../../../change-audit/CA-077-fix-desktop-chat-turn-skills-summary-visibility.md)
- Replaces: `none`
- Tags: `desktop, bugfix, chat, skills, composer, regression`

## AI Quick View

### Summary

- The Task-053 selected-skills chip existed in `ChatInput`, but slash-triggered skill selection could leave the picker in command mode instead of making the below-input chip the visible handoff.
- The collapsed chip was also too visually quiet to read reliably against the composer background.
- The fix closes slash mode after a slash pick, preserves grouped-picker multi-select behavior, and makes the collapsed chip show a compact skill preview.

### Current Ask

- Done. Selected turn skills are now visibly surfaced below the chat input after slash selection and remain inspectable/removable from the expanded chip.

### Key Decisions

- `V-1` Slash-triggered skill selection should clear only the slash command text and close the picker so the selected-skills summary becomes the visible state.
- `V-2` Toolbar-triggered skill selection keeps the picker open so multi-select behavior from Task-048 remains intact.
- `V-3` The collapsed summary should include a compact `/skill` preview and stronger styling so users can see which skills are armed.

### Constraints

- Desktop normal chat mode only.
- UI interaction fix only; no runner, provider, or store contract changes.
- Preserve BUG-062 prompt-preservation behavior for grouped picker closes.

### Open Questions

- None.

### Source Refs

- `Task-053`
- `Task-048`
- `Task-051`
- `BUG-062`
- `BUG-063`
- `CA-070`

## 1. Issue Summary

After Task-053, users expected selected skills to be visible below the chat prompt before sending a direct chat turn. The chip was implemented, but slash-triggered selection could leave the UI in slash picker mode and the collapsed summary styling was too subtle, making the selected-skills UI appear missing.

## 2. Parent Links

- impacted coding plan: [Task-053: Desktop Chat Turn Skills Summary Chip](../../08-Task/done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md)
- impacted tech design: [Task-048: Desktop Chat Skills Workflow Composer Alignment](../../08-Task/done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot`, normal chat mode
- reproduction steps:
  - select a project and provider in desktop chat mode
  - type `/` in the chat input to open the skill picker
  - choose a skill from the slash picker
  - look below the input for the Task-053 selected-skills summary
- frequency: reproducible for the slash-pick path before the fix

## 4. Expected vs Actual

- expected:
  - choosing a slash-picker skill arms the skill for the current turn
  - the slash command exits and the selected-skills summary is visible below the input
  - the grouped skills button can still be used for multi-select picking without closing after every click
- actual:
  - the slash picker remained the dominant UI state after selection
  - the collapsed selected-skills summary had low contrast and no skill-name preview

## 5. Impact

- users affected: desktop chat users selecting skills for direct provider turns
- workflows affected: normal chat skill selection and skill-injection confidence before send
- severity: medium

## 6. Root Cause

- hypothesis:
  - slash-triggered selection did not transition from command-pick mode into the selected-summary state
  - the collapsed chip styling was too understated to serve as a reliable visible confirmation
- confirmed cause:
  - `pickSkill` only updated `selectedSkills`; it did not clear `text` or close the picker when `slashQuery` was active
  - `.turn-skills-summary` used dim text, a quiet border, and no visible selected-skill names
- evidence:
  - `apps/desktop-flowpilot/src/components/ChatInput.tsx`
  - `apps/desktop-flowpilot/src/styles.css`

## 7. Fix Strategy

- `F-1` When `pickSkill` runs from slash mode, clear the slash command text and close the picker.
- `F-2` Leave grouped skill-button selection behavior unchanged so users can still select multiple skills in one picker session.
- `F-3` Add a compact preview of the first selected skill names to the collapsed summary chip.
- `F-4` Increase the chip width, contrast, and border treatment so the below-input state is visible.

## 8. Validation

- `V-1` `npm run typecheck --prefix apps/desktop-flowpilot`
- `V-2` `npm run build --prefix apps/desktop-flowpilot`
- `V-3` `git diff --check`
- `V-4` Browser smoke attempt: Vite renderer opened, but the current desktop bootstrap required local auth before the chat surface, so manual visual verification of the authenticated chat composer was not completed in this session.

## 9. Regression Guard

- tests: desktop TypeScript build and production renderer/electron build
- alerts: none
- audit checks: GitNexus impact reviewed for `ChatInput`, `pickSkill`, and `removeSkill`; all reported LOW risk

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - no runner/provider contract changed
  - no automated browser test harness was added for authenticated desktop chat in this slice
