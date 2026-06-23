# Task-087: Chat Slash Commands For Skill And Agent UI

## Metadata

- Document ID: `Task-087`
- Title: `Chat Slash Commands For Skill And Agent UI`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [Task-083: Desktop Agents Panel And Focus Navigation](./Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [CA-120: Agent spawn UX hardening](../../../change-audit/CA-120-agent-spawn-ux-hardening.md)
- Replaces: `None`
- Tags: `desktop, chat-input, slash-command, skills, spawn-agent, ui`

## AI Quick View

### Summary

- The chat input already opens a skill picker when the user types `/`.
- Add two explicit slash sub-commands: `/s` opens the skill picker UI, and `/a` opens the spawn-agent UI (the same panel used in the right sidebar).
- `/a` reuses the existing `openAgentSpawnGuide` store action so there is one spawn UI, not a duplicate.

### Current Ask

- Wire `/s` → skill UI and `/a` → spawn-agent UI in the chat composer, reusing existing pickers/panels.

### Key Decisions

- `T-1` The leading command letter after `/` selects the command: `a` → agent, anything else (incl. `s` and bare `/`) → skill picker. The command letter is not treated as a search term.
- `T-2` `/a` strips the slash fragment from the prompt, restores the caret, and calls `openAgentSpawnGuide()` (the existing right-sidebar spawn panel).
- `T-3` `/s` opens the skill picker showing the full list (the leading `s` is not used as a filter); further typing still searches.

### Constraints

- Do not introduce a second spawn UI; reuse `openAgentSpawnGuide`.
- Preserve existing `/` skill-search behavior and `@agent` mention routing.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — `findActiveSlash`, `slashFragment`, picker render, key handling.
- `apps/desktop-flowpilot/src/state/store.ts` — `openAgentSpawnGuide` / `agentSpawnGuideOpen`.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — spawn panel opened by `agentSpawnGuideOpen`.

## 1. Goal

Let users open the skill picker with `/s` and the spawn-agent panel with `/a` directly from the chat composer, reusing the existing UIs.

## 2. Parent Links

- coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- system spec: [SS-06](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)
- specific upstream ids: CP-19 P-7 (desktop agent UX)

## 3. Trigger

The composer already supports `/` for skills. Users asked for an explicit `/a` to open the spawn-agent UI (and `/s` for the skill UI) so spawning is reachable without leaving the keyboard, using the same panel as the right sidebar.

## 4. Exact Change

- `T-1` Classify the active slash fragment: `query === "a"` → `agent` command; otherwise `skill`.
- `T-2` Gate the skill picker so it does not show for the `agent` command; render a small one-action "Spawn sub-agent" popover instead.
- `T-3` On Enter (or click) in the `agent` command, strip the `/a` fragment, restore the caret, and call `openAgentSpawnGuide()`.
- `T-4` Treat `/s` as "open skill UI" (search reset to empty) rather than a search for "s".
- `T-5` Exclude the `agent` command from `canSend` so `/a` is not sent as a prompt.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`
- modules: chat composer slash handling
- routes: none
- tables: none

## 6. Acceptance Check

- Typing `/s` opens the skill picker with the full list; typing `/a` shows the spawn-agent action and Enter opens the spawn panel (same as the right sidebar), with the `/a` text removed.
- `/` skill search and `@agent` mention routing still work.
- `npx tsc --noEmit` passes.

## 7. Out of Scope

- New spawn UI or backend spawn changes.
- Additional slash commands beyond `/s` and `/a`.

## 8. Completion Notes

- result: implemented in `ChatInput.tsx`; `/a` reuses `openAgentSpawnGuide`; `/s` opens the skill UI; type-check green.
- follow-ups: the initial implementation only triggered the agent command on the exact one-character query `"a"`, so `/agent` and other forms fell through to the skill picker and the command appeared not to work — fixed in [BUG-134](../../09-BugFix/done/BUG-134-Slash-A-Command-Does-Not-Open-Spawn-Agent-UI.md) (trigger broadened to `"a"`/`"agent"` and `"s"`/`"skill"`). The desktop vitest suite fails to load on this machine with a pre-existing ESM config error, and the Electron app was not run here, so `/a` end-to-end needs user retest.
- upstream docs updated: none required; behavior is additive to the existing composer.
