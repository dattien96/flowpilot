# BUG-107: Spawn Dialog Shows CODEX + CLAUDE Dual Badges For All Agents

## Metadata

- Document ID: `BUG-107`
- Title: `Spawn Dialog Shows CODEX + CLAUDE Dual Badges For All Agents`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-104](BUG-104-Agents-Panel-Spawn-Button-Badge-And-Description-Regressions.md)
- Replaces: `None`
- Tags: `agents-panel, spawn-dialog, provider-badge, regression, desktop`

## AI Quick View

### Summary

- The spawn agent dialog shows CODEX + CLAUDE dual provider badges for every agent in the list — including project-local agents loaded from `.claude/agents` and `.codex/agents`.
- Root cause: the `isProviderAgnostic` condition in `AgentsPanel.tsx` was written as `!agent.provider || (agent.source === "flowpilot" && !agent.provider)`, which simplifies to `!agent.provider`. Any agent without an explicit `provider:` field in its frontmatter (the common case) evaluates to true, incorrectly triggering the dual-badge path.
- The dual badge is only correct for built-in FlowPilot agents (`source === "flowpilot"`) that carry no provider restriction, meaning they can run on either Codex or Claude.
- Project-local agents from `.claude/agents` (`source === "claude"`) and `.codex/agents` (`source === "codex"`) should show a single badge derived from their source directory, regardless of whether they declare a `provider:` field.

### Current Ask

- Fix `isProviderAgnostic` to `agent.source === "flowpilot" && !agent.provider` so only built-in agnostic agents get dual badges.

### Key Decisions

- `V-1` Project-local Claude agents (`source === "claude"`) always show CLAUDE badge; Codex agents (`source === "codex"`) always show CODEX badge — independent of their frontmatter `provider` field.
- `V-2` Provider-home agents (`source === "provider"`) continue to derive badge from path (`.claude` substring → CLAUDE, otherwise CODEX).
- `V-3` Built-in agents (`source === "flowpilot"`) with no explicit `provider` field show both badges, since they are genuinely runnable on any provider.

### Constraints

- Fix is UI-only; no backend change required.
- The `isProviderAgnostic` logic is used only inside the spawn dialog agent list; no other code path is affected.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx` — `isProviderAgnostic` expression (introduced in the same session as BUG-104 fix)

## 1. Issue Summary

After the BUG-104 session introduced dual-badge support for provider-agnostic agents, all agents in the spawn dialog showed both CODEX and CLAUDE badges — including agents loaded from `.claude/agents` (which should show CLAUDE only) and `.codex/agents` (which should show CODEX only). The condition `!agent.provider` is true for any agent whose markdown frontmatter does not include a `provider:` line, which is the normal case for project-local agents that rely on their directory location to signal provider affinity.

## 2. Parent Links

- coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- tech design: [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- system spec: Task-083 is the originating task for the Agents Panel UI

## 3. Environment and Reproduction

- environment: Desktop app, any project with `.claude/agents` and/or `.codex/agents` files
- reproduction steps:
  1. Open any project that has agent files in `.claude/agents` or `.codex/agents`.
  2. Click "+ Spawn agent" in the Agents panel.
  3. Observe that every agent in the list shows both CODEX and CLAUDE badges regardless of source.
- frequency: Consistent — affects every project-local agent without an explicit `provider:` frontmatter field

## 4. Expected vs Actual

- expected:
  - Built-in agents (`source === "flowpilot"`, no explicit `provider`) → CODEX + CLAUDE dual badges
  - Project-local `.claude/agents` agents (`source === "claude"`) → CLAUDE badge only
  - Project-local `.codex/agents` agents (`source === "codex"`) → CODEX badge only
  - Provider-home agents (`source === "provider"`) → badge derived from path
- actual: All agents without a `provider:` frontmatter field show CODEX + CLAUDE dual badges

## 5. Impact

- users affected: All users with project-local agent files; the badge is misleading and implies cross-provider support that may not be true
- workflows affected: Spawn dialog agent selection — provider selection intent is unclear
- severity: Medium — visual regression only; spawning still works correctly; badge is cosmetic guidance

## 6. Root Cause

- hypothesis: `isProviderAgnostic` evaluates `!agent.provider` first, which short-circuits the entire OR expression.
- confirmed cause:
  ```tsx
  // WRONG: reduces to !agent.provider for any agent without provider field
  const isProviderAgnostic = !agent.provider ||
    (agent.source === "flowpilot" && !agent.provider);
  ```
  The first operand `!agent.provider` is `true` for all agents that lack an explicit `provider:` frontmatter field. The entire OR is then trivially true for those agents, including all `.claude/agents` and `.codex/agents` files that simply omit the field.
- evidence: Screenshot from user's project showing `.claude/agents/` and `.codex/agents/` agents both rendering dual badges; logical reduction of the expression confirms the cause.

## 7. Fix Strategy

- `F-1` Replace the `isProviderAgnostic` expression in `AgentsPanel.tsx`:
  ```tsx
  // BEFORE (wrong):
  const isProviderAgnostic = !agent.provider ||
    (agent.source === "flowpilot" && !agent.provider);

  // AFTER (correct):
  const isProviderAgnostic = agent.source === "flowpilot" && !agent.provider;
  ```
  This restricts the dual-badge path to only FlowPilot built-ins with no explicit provider, which is the correct semantic: built-ins are provider-agnostic by design; project-local agents get their provider from the directory they live in.

## 8. Validation

- `V-1` TypeScript type-check (`npx tsc --noEmit`) passes with no errors.
- `V-2` Visual inspection: project-local `.claude/agents` agents now show CLAUDE badge only; `.codex/agents` agents show CODEX badge only; built-in agents (coder, reviewer, tester with no `provider` set) show both badges.
- `V-3` No unit tests are affected — the badge logic has no dedicated test; visual regression guard relies on the component rendering.

## 9. Regression Guard

- tests: No dedicated test for this badge logic exists; the correct expression is simple enough that future readers can verify by inspection.
- alerts: None.
- audit checks: `isProviderAgnostic` in `AgentsPanel.tsx` must check `agent.source === "flowpilot"` in addition to `!agent.provider` — the source check is load-bearing.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — the badge rendering rule is not documented outside this file.
- notes left unchanged on purpose: The `isClaudeSource` derivation is unchanged; it correctly handles `source === "claude"` and `source === "provider"` with `.claude` path substring.
