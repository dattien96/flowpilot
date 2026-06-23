# BUG-104: Agents Panel Spawn Button, Badge, and Description Regressions

## Metadata

- Document ID: `BUG-104`
- Title: `Agents Panel Spawn Button, Badge, and Description Regressions`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-22`
- Last Updated: `2026-06-22`
- Parent Documents: [Task-083: Desktop Agents Panel And Focus Navigation](../../08-Task/inprogress/Task-083-Desktop-Agents-Panel-And-Focus-Navigation.md), [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- Child Documents: `None`
- Related Documents: [BUG-099](BUG-099-Desktop-Spawn-Agent-Does-Not-Surface-Child-Runs-In-Agents-Panels.md), [BUG-101](BUG-101-Restarted-Desktop-Session-Did-Not-Show-The-Spawn-Agent-Child-Run.md)
- Replaces: `None`
- Tags: `multi-agent, agents-panel, ui, regression, desktop`

## AI Quick View

### Summary

- The `+ Spawn agent` button in the Agents panel sidebar had no disabled CSS rule, so it appeared clickable (with `var(--accent)` styling) even when `client.spawnAgent` was unavailable — clicking opened the modal but spawning silently no-opped.
- Agent cards in the spawn modal hardcoded `CODEX` as the provider badge regardless of where the agent definition was loaded from; agents sourced from `.claude/agents` directories displayed an incorrect CODEX tag.
- Agent descriptions in the spawn modal had no line-clamp, causing long system-prompt-derived descriptions to overflow card height and produce unreadable walls of text.

### Current Ask

- Grey out the `+ Spawn agent` button when `client.spawnAgent` is unavailable.
- Add `.spawn:disabled` CSS so the button visually signals the disabled state.
- Derive the CLAUDE/CODEX badge in the modal from `agent.source` (and `agent.path` for `"provider"` sourced agents).
- Clamp description text to 3 lines with ellipsis in the spawn modal agent cards.

### Key Decisions

- `V-1` Badge derivation uses `source === "claude"` → CLAUDE; `source === "provider" && path.includes(".claude")` → CLAUDE; otherwise CODEX. This avoids adding a new field and matches how `agent_catalog.go` sets source.

### Constraints

- `AgentRunSummary` does not carry a `providerKey` field, so running-agent cards in the sidebar panel remain with a CODEX placeholder badge — fixing those requires a backend schema change (out of scope here).
- Description clamp uses `-webkit-line-clamp: 3` which is broadly supported across Chromium (Electron) and modern browsers.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/AgentsPanel.tsx`
- `apps/desktop-flowpilot/src/styles.css`
- `apps/local-runner/internal/runner/agent_catalog.go` — `source` field semantics confirmed

## 1. Issue Summary

Three cosmetic/functional regressions in the Agents Panel introduced by Task-083:

1. **Spawn button always appears active**: `disabled={!mainRunId}` was the only guard. When `client.spawnAgent` is `undefined` (e.g., MockRunnerClient without the method, or a provider that does not implement it), the button renders with full accent color and a pointer cursor. Clicking opens the spawn modal, but hitting "Spawn ▸" there silently returns early because `spawn()` guards `if (!client.spawnAgent) return`. The user sees no feedback.

2. **Wrong provider badge in modal**: Every agent card in the spawn dialog showed `CODEX` badge (`prov-codex` class). Agents discovered from `.claude/agents` paths (project-local or provider-home) have `source === "claude"` or `source === "provider"` with `.claude` in the path. These should show the `CLAUDE` badge (`prov-claude` class, orange color).

3. **Untruncated descriptions**: Agent descriptions derived from the system-prompt body (used when no `description:` frontmatter key is present) can be very long. No CSS clamp was applied to `.opt .ds`, so the modal card expanded to show the full text — hundreds of characters in some cases.

## 2. Parent Links

- impacted coding plan: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md)
- impacted tech design: Task-083 is the implementation task; no separate SD document for the Agents panel UI
- impacted system spec: `SS-13` (AI-Followable Document Contract — no behavior change, cosmetic fixes only)

## 3. Environment and Reproduction

- environment: Desktop app (Electron/Vite), all platforms
- reproduction steps:
  1. Open the app without an active run — `client.spawnAgent` may be undefined in some client modes. Observe the `+ Spawn agent` button renders in full blue accent style, not greyed.
  2. Start the spawn modal, scroll the agent list — agents from `.claude/agents` show `CODEX` badge instead of `CLAUDE`.
  3. Select an agent whose description was auto-derived from system prompt (e.g., `chatbot-builder`) — full multi-paragraph text renders without truncation.
- frequency: Always (deterministic styling/logic bug)

## 4. Expected vs Actual

- expected:
  - `+ Spawn agent` button is greyed out (opacity 0.5, `var(--border)` color) when `client.spawnAgent` is not defined.
  - Agents from `.claude/agents` show an orange `CLAUDE` badge.
  - Description text in modal cards is clamped at 3 lines with `…` overflow.
- actual:
  - Button appeared fully active regardless of `client.spawnAgent` availability.
  - All agents showed `CODEX` badge.
  - Full description text overflowed card height.

## 5. Impact

- users affected: All desktop users who open the Agents panel / Spawn modal
- workflows affected: Sub-agent spawning flow (Task-082 / Task-083)
- severity: Low-Medium — cosmetic and UX confusion; no data loss

## 6. Root Cause

- hypothesis: Hardcoded badge strings and missing CSS disabled rule in the Task-083 initial implementation.
- confirmed cause: 
  1. `AgentsPanel.tsx` line 187: `disabled={!mainRunId}` did not check `client.spawnAgent`.
  2. No `.spawn:disabled` CSS rule existed.
  3. Lines 247-248 (modal map): `cardProvBadge = "CODEX"` and `cardProvClass = "codex"` were hardcoded.
  4. `.opt .ds` CSS block had no `overflow`/`-webkit-line-clamp` properties.
- evidence: Code inspection; `agent_catalog.go` confirms `source` is `"claude"` for project-local `.claude/agents` entries and `"provider"` (with `.claude` in path) for provider-home agents.

## 7. Fix Strategy

- `F-1` Add `.spawn:disabled` CSS rule: `border-color: var(--border); color: var(--text-dim); opacity: 0.5; cursor: default`.
- `F-2` Change `disabled` prop on spawn button to `!mainRunId || !client.spawnAgent`.
- `F-3` Add `-webkit-line-clamp: 3` with `display: -webkit-box; overflow: hidden` to `.opt .ds`.
- `F-4` Derive `cardProvBadge` / `cardProvClass` from `agent.source` and `agent.path`: `source === "claude"` → CLAUDE; `source === "provider" && path.includes(".claude")` → CLAUDE; else CODEX.
- `F-5` Mirror `F-2` and `F-4` in `.phase1-tests` compiled JS counterpart.

## 8. Validation

- `V-1` TypeScript typecheck (`tsc --noEmit`) passes with no errors.
- `V-2` `node --test` on `.phase1-tests/apps/desktop-flowpilot/src/components/AgentsPanel.test.js` passes (1/1).
- `V-3` Visual inspection: spawn button grey when no active run (no `mainRunId`).
- `V-4` Visual inspection: agents from `.claude/agents` show orange CLAUDE badge in modal.
- `V-5` Visual inspection: long description text clamps at 3 lines with `…`.

## 9. Regression Guard

- tests: `AgentsPanel.test.js` covers `formatDependencyLabels`; badge derivation and disabled state are covered by visual inspection (no new unit tests added — render tests require a React test environment not present in the phase1 node:test harness).
- alerts: None.
- audit checks: See `agent_catalog.go` — `source` field semantics are stable; badge logic is keyed to those values.

## 10. Follow-Up Document Updates

- upstream docs that must change: None — these are purely UI rendering fixes with no behavior or contract changes.
- notes left unchanged on purpose: `AgentRunSummary` does not expose `providerKey`; the running-agent sidebar cards therefore keep a CODEX placeholder badge until the backend schema is extended (separate task).
