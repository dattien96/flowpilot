# Task-053: Desktop Chat Turn Skills Summary Chip

## Metadata

- Document ID: `Task-053`
- Title: `Desktop Chat Turn Skills Summary Chip`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-051: Desktop Chat Empty Project Guard And Control Header Cleanup](../done/Task-051-Desktop-Chat-Empty-Project-Guard-And-Control-Header-Cleanup.md), [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- Replaces: `none`
- Tags: `desktop, ui, chat, skills, composer`

## AI Quick View

### Summary

- Add a small collapsible summary chip below the chat input bar that appears whenever the user has one or more skills selected for the current turn.
- The chip collapses to a single summary line ("N skills selected") and expands to show each selected skill as a removable pill.
- Modeled visually on the tool-call group summary in the Timeline component, keeping the UI non-intrusive.
- Change is purely additive UI; no store contract, runner, or provider changes.

### Current Ask

- Done. The turn-skills summary chip renders below the textarea when skills are selected, defaults collapsed, and allows removal from the expanded view.

### Key Decisions

- `T-1` Default the summary to collapsed so the chip stays small and does not shift the composer layout.
- `T-2` Reuse `removeSkill` from the existing skill picker to keep skill removal logic centralized.
- `T-3` Disable remove buttons when the runner is blocked (same guard as the rest of the controller).

### Constraints

- Normal chat mode only (`isChatMode`).
- UI only; no backend, runner, or store contract changes.
- Preserve all existing chat controller behavior from `Task-044` through `Task-052`.

### Open Questions

- None for this slice.

### Source Refs

- `Task-051`
- `Task-048`
- `SS-11`

## 1. Goal

Surface a compact, always-visible indicator directly below the chat prompt whenever skills are active for the current turn. The user can expand it to inspect or remove individual skills without reopening the full skill picker.

## 2. Parent Links

- coding plan: [Task-051: Desktop Chat Empty Project Guard And Control Header Cleanup](../done/Task-051-Desktop-Chat-Empty-Project-Guard-And-Control-Header-Cleanup.md)
- tech design: [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `Task-048 T-2`, `Task-051`, `SS-11`

## 3. Trigger

After the skills picker was compressed into a single grouped selector (Task-048), the user had no persistent at-a-glance indicator showing which skills were armed for the next send. The request was to add a small non-intrusive chip below the chat prompt — modeled after the tool-call UI — so selected skills are always visible without crowding the composer.

## 4. Exact Change

- `T-1` Add `skillSummaryExpanded` local state (default `false`) to `ChatInput.tsx`.
- `T-2` Render a `turn-skills-bar` block below `input-bar` when `isChatMode && selectedSkills.length > 0`. The block contains a toggle button (`turn-skills-summary`) and a conditional expanded body (`turn-skills-body`) listing skill pills with remove buttons.
- `T-3` Add CSS classes `turn-skills-bar`, `turn-skills-summary`, `turn-skills-summary-open`, `turn-skills-caret`, `turn-skills-title`, `turn-skills-body`, `turn-skill-row`, `turn-skill-name`, `turn-skill-remove` to `styles.css`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- In normal chat mode with no skills selected the chip is invisible.
- When one or more skills are selected the chip appears below the textarea, collapsed, showing "N skill(s) selected".
- Clicking the chip expands it to list each selected skill as a removable pill.
- Clicking a pill's remove button (✕) deselects that skill via the existing `removeSkill` handler.
- Remove buttons are disabled while the runner is blocked.
- Collapsing and re-expanding does not affect the underlying selected skills state.
- No existing chat controls regress.

## 7. Out of Scope

- Workflow / step mode chip.
- Persisting expanded/collapsed preference across sessions.
- Skill reordering from the chip.
- Any changes to runner, store, or provider contracts.

## 8. Completion Notes

- result: Implemented `turn-skills-bar` in `ChatInput.tsx` and added companion CSS in `styles.css`. TypeScript typecheck passes with no errors.
- follow-ups: none
- upstream docs updated: `Task-053`
