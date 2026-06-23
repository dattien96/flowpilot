# BUG-134: Slash /a Command Does Not Open Spawn-Agent UI

## Metadata

- Document ID: `BUG-134`
- Title: `Slash /a Command Does Not Open Spawn-Agent UI`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [Task-087: Chat Slash Commands For Skill And Agent UI](../../08-Task/done/Task-087-Chat-Slash-Commands-For-Skill-And-Agent-UI.md), [CA-121: Agent panel runtime fixes](../../../change-audit/CA-121-agent-panel-runtime-fixes.md)
- Replaces: `None`
- Tags: `desktop, chat-input, slash-command, spawn-agent, ui`

## AI Quick View

### Summary

- Task-087's `/a` slash command did not reliably open the spawn-agent UI: the agent command only triggered on the exact one-character query `"a"`, so `/agent` (or any extra character) fell through to the skill picker.
- A second defect: a bare `/` immediately popped the skill UI (the legacy behavior). With the new `/a` namespace, the skill UI should require an explicit `/s` instead of opening on any `/`.
- Fix: namespace slash input by the command letter after `/`. `/a…` (or `/agent`) → spawn-agent UI; `/s…` (or `/skill`) → skill picker, with the text after `/s` as the search term; a bare `/` (or any other leading letter) opens nothing. The agent command strips the fragment and opens the spawn panel via `openAgentSpawnGuide` (the same UI as the right sidebar), confirmable by Enter or click.

### Current Ask

- Make `/a` (and `/agent`) open the spawn-agent UI; make the skill UI require `/s` (and `/skill`) instead of opening on a bare `/`.

### Key Decisions

- `V-1` Slash classification is by the command letter after `/`: leading `a` → agent, leading `s` → skill, anything else (including bare `/`) → no picker.
- `V-2` Under `/s`, the leading `s` (or the full word `skill`) is the command; the remainder is the skill search term.
- `V-3` The agent command reuses `openAgentSpawnGuide` (no duplicate spawn UI).

### Constraints

- Bare `/` must no longer open the skill UI; a command letter is required.
- Do not break `@agent` mention routing or the skill button (which opens the picker directly).
- Do not introduce a second spawn UI.

### Open Questions

- Could not run the Electron desktop app in this environment to confirm the panel opens end-to-end; needs user retest.

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatInput.tsx` — `slashCommand` classification, `triggerAgentSlash`, agent popover, picker-search sync.
- `apps/desktop-flowpilot/src/state/store.ts` — `openAgentSpawnGuide`.
- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — modal opened by `agentSpawnGuideOpen`.

## 1. Issue Summary

`/a` was matched only when the active slash fragment query was exactly `"a"`. Typing `/agent`, `/a ` (space ends the slash scan), or any additional character routed to the skill picker instead, so the spawn-agent command appeared not to work. Separately, a bare `/` immediately opened the skill picker (legacy behavior); now that `/a` exists, the skill UI should open only under an explicit `/s` so the two namespaces are distinct.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) (P-7)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- impacted system spec: [SS-06](../../05-System-Specs/SS-06-Workflow-Skill-Agent.md)

## 3. Environment and Reproduction

- environment: FlowPilot desktop chat composer with a provider selected.
- reproduction steps:
  1. Type `/agent` (or `/a ` with a trailing space) → the skill picker shows instead of the spawn-agent command.
  2. Type a bare `/` → the skill UI pops immediately (should require `/s`).
- frequency: always.

## 4. Expected vs Actual

- expected: `/a`/`/agent` surface the spawn-agent command and open the spawn panel; `/s`/`/skill` open the skill UI; a bare `/` opens nothing.
- actual: only the exact `"a"` triggered the agent command; a bare `/` opened the skill UI.

## 5. Impact

- users affected: anyone using the `/a` shortcut.
- workflows affected: keyboard-driven agent spawning.
- severity: Low-Medium — feature effectively unreachable via the natural `/agent` form.

## 6. Root Cause

- confirmed cause: (1) the classification compared the query to the single literal `"a"`, so longer/space-terminated queries never matched; (2) `showPicker` was driven by "any active slash fragment", so a bare `/` opened the skill UI.

## 7. Fix Strategy

- `F-1` Classify the slash by the command letter after `/`: leading `a` → agent, leading `s` → skill, anything else (incl. bare `/`) → no picker.
- `F-2` Drive `showPicker` from `slashCommand === "skill"` (or the skill button), not from "any slash fragment", so a bare `/` no longer opens it.
- `F-3` Under `/s`, strip the leading `s` (or full word `skill`) and use the remainder as the skill search term.
- `F-4` Keep the agent popover (Enter/click) that strips the fragment and calls `openAgentSpawnGuide`.

## 8. Validation

- `V-1` `npx tsc --noEmit` (desktop) — pass.
- `V-2` Could NOT run the Electron + Go-runner desktop app in this environment, and the desktop vitest suite fails to load with a pre-existing ESM config error, so this fix is verified by type-check and code inspection only. User retest: `/a`/`/agent` opens the spawn-agent panel; `/s` opens the skill list and `/sgit` filters it; a bare `/` opens nothing.

## 9. Regression Guard

- tests: type-check; manual retest documented above.
- alerts: a bare `/` opening the skill UI, or `/agent` showing the skill picker, indicates the classification regressed.
- audit checks: the agent command must reuse `openAgentSpawnGuide`, not a new UI.

## 10. Follow-Up Document Updates

- upstream docs that must change: Task-087 completion notes updated to reference this fix and the verification limitation.
- notes left unchanged on purpose: `/` skill search and `@agent` routing are unchanged.
