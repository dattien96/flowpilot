# Task-079: Chat Skill Picker Keyboard Command Flow

## Metadata

- Document ID: `Task-079`
- Title: `Chat Skill Picker Keyboard Command Flow`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [Task-053: Desktop Chat Turn Skills Summary Chip](../done/Task-053-Desktop-Chat-Turn-Skills-Summary-Chip.md), [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md)
- Child Documents: `none`
- Related Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SS-06: Workflow Skill Agent](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)
- Replaces: `none`
- Tags: `desktop, ui, chat, skills, keyboard, ux, command-mode`

## AI Quick View

### Summary

- `/` now triggers the skill picker at **any cursor position** in the textarea (not only as the first character), by scanning backwards from the cursor to find a valid slash fragment (preceded by space or at string start).
- When a skill is selected via the slash command mode, the `/skillname` fragment in the prompt text is **replaced with the skill name** in-place, keeping the surrounding prompt text intact.
- Full **keyboard navigation**: Arrow Up/Down moves the highlight through the filtered list; Enter selects the highlighted item; Escape dismisses; Space or moving cursor away naturally closes the picker without clearing text.
- UI-only change. No store contract, runner, or provider behavior modified.

### Current Ask

- Done. T-1 through T-5 implemented and TypeScript check passes.

### Key Decisions

- `T-1` Use `findActiveSlash(text, cursor)` — scan backwards from cursor position to find an active `/query` fragment; valid only if `/` is at position 0 or preceded by a space. This prevents false triggers on URL-like strings (e.g. `http://`).
- `T-2` Replace the old `text.startsWith("/")` guard with a cursor-aware `useMemo` so the picker responds to `/` at any text position.
- `T-3` On skill selection in command mode, splice the skill name into the text at the slash fragment position instead of clearing the entire input.
- `T-4` Track cursor position via `cursorPos` state updated in textarea `onChange` and `onSelect`; this drives the fragment detector.
- `T-5` `pickerHighlightIndex` state drives arrow-key navigation; resets to -1 on each query change.

### Constraints

- Normal chat mode only (`isChatMode`).
- Skills selected via the button-click Skill box retain old silent-select behavior (no text injection).
- No backend, runner, or store contract changes.

### Open Questions

- None.

### Source Refs

- `Task-053`, `Task-048`, `Task-044`

## 1. Goal

Deliver a fully keyboard-driven slash-command flow for skill selection in the chat input: type `/` anywhere in the prompt to open the picker, filter by continuing to type, navigate with arrow keys, confirm with Enter, and have the skill name replace the `/query` fragment in the prompt.

## 2. Parent Links

- coding plan: closest parent is the desktop chat task chain (Task-044 → Task-048 → Task-053)
- tech design: SS-06 Workflow Skill Agent (skill registry and execution rules)
- system spec: SS-06
- specific upstream ids: `Task-053` (turn-skills summary chip), `Task-048` (skills composer alignment)

## 3. Trigger

User reported that `/` only activated the skill picker when typed as the very first character. Prompts like `use /cook to do X` did not trigger the picker. Additionally, there was no keyboard navigation — selecting a skill required mouse interaction — and selecting a skill in slash mode cleared the entire prompt instead of inserting the skill name.

## 4. Exact Change

- `T-1` Add `findActiveSlash(text, cursor)` helper before `ChatInput` component. Scans backwards from cursor to find a valid `/query` fragment. Returns `{ index, query }` or `null`.
- `T-2` Replace `const slashQuery = isChatMode && text.startsWith("/") ? ...` with a `useMemo` that calls `findActiveSlash(text, cursorPos)`, producing `slashFragment` and deriving `slashQuery` from it. `showPicker` now depends on `slashFragment !== null` instead of `slashQuery !== null`.
- `T-3` Add `cursorPos: number` and `pickerHighlightIndex: number` state; add `textAreaRef` for programmatic focus/cursor restore after selection.
- `T-4` Update `pickSkill`: when `slashFragment !== null`, splice skill name into text at fragment position instead of clearing; restore textarea focus and cursor position via `setTimeout`.
- `T-5` Extend `onKeyDown`: ArrowDown/ArrowUp move `pickerHighlightIndex`; Enter selects highlighted item; Escape dismisses and resets cursor tracking. Update textarea `onChange` and `onSelect` to maintain `cursorPos`. Update skill item rendering to accept `idx` and apply `skill-item-highlighted` class.
- `T-6` Remove all `setText("")` calls from dismiss paths (outside-click, close button). Replace with `setCursorPos(0)` to invalidate the fragment without wiping user text.
- `T-7` Add `.skill-item-highlighted` CSS rule: `background: var(--bg-2); outline: 1px solid var(--accent)`.
- `T-8` Update placeholder text: `"Type a message. Use / anywhere to pick a skill."`.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: Chat input composer, skill picker UI
- routes: n/a
- tables: n/a

## 6. Acceptance Check

- Typing `/` anywhere in the textarea (mid-sentence after a space, or at start) opens the skill picker and focuses the search.
- Continuing to type after `/` filters the list in real time.
- Arrow Down/Up moves the highlight through filtered items without mouse.
- Enter selects the highlighted item: `/query` in the text is replaced with the skill name; focus returns to textarea.
- Escape dismisses the picker without clearing the prompt.
- Space typed after `/` dismisses the picker (the scan hits the space and returns null).
- Clicking outside the component closes the picker without clearing the prompt.
- Selecting a skill via the Skill box button (not slash) still silently adds it to `selectedSkills` with no text change.
- TypeScript check passes with no errors.

## 7. Out of Scope

- Keyboard navigation when the picker is open via the button (arrow key nav is only active when `slashFragment !== null`).
- Removing skills from the prompt text when deselected via slash mode.
- Auto-triggering the picker on `Tab` keypress (not requested in this slice).
- Any backend, runner, or store contract changes.

## 8. Completion Notes

- result: Implemented in a single pass. TypeScript check passes (`npx tsc --noEmit` exits 0).
- follow-ups: Arrow-key navigation for button-opened picker could be added in a follow-up if desired.
- upstream docs updated: none required (UI-only change).
