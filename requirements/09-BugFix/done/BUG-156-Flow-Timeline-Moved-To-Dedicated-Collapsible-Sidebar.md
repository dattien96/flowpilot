# BUG-156: Flow Timeline Moved To Dedicated Collapsible Sidebar

## Metadata

- Document ID: `BUG-156`
- Title: `Flow Timeline Moved To Dedicated Collapsible Sidebar`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-153-Flow-Mode-Right-Sidebar-Missing-Step-Execution-State.md`, `requirements/09-BugFix/done/BUG-155-Flow-Mode-Sidebar-Shows-Generic-Step-Label-Not-Node-Identity.md`
- Child Documents: `none`
- Related Documents: `none`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, workflow-steps-runtime`

## AI Quick View

### Summary

- `BUG-153`'s "Workflow Steps" panel rendered as a flat card list stacked inside the existing right sidebar (`right-sidebar-stack`, alongside `AgentsPanel`/`ProviderAccountsPanel`), competing for space with unrelated panels even though it is only relevant during an active Flow Mode run.
- The user asked for a vertical connected-circle stepper (reference: a mobile "Step One / Step Two / ..." timeline mockup with filled/hollow circles joined by a line) instead of the flat card list, with distinct icon states for running/complete/idle/needs-approval/error, and the whole row highlighted for the current step.
- The user also asked to move this component into its own dedicated collapsible sidebar between the chat column and the existing right sidebar, visible only while Flow Mode is running, with a collapsed icon-only rail and an expandable full view with a progress summary.

### Current Ask

- Replace the flat step-card list with a vertical connected-circle timeline (5 icon states, current-step row highlight).
- Extract it into a new collapsible sidebar positioned between the chat area and the existing right sidebar, visible only when `chatMode` is Flow Mode (`workflow_step_auto`) and the run status is `running`; collapsed = icon rail only, expanded = progress summary + full per-step detail.

### Key Decisions

- `F-1` New presentational component `FlowStepTimeline` (`components/FlowStepTimeline.tsx`) renders the connected-circle stepper; a `compact` prop switches between the icon-only rail and the full title/description/meta view — one component serves both the collapsed and expanded sidebar states rather than duplicating markup.
- `F-2` Visual state is derived from `WorkflowStepRuntimeStatus` into one of five states (`idle`/`running`/`done`/`approval`/`error`); the current step (`running` or `waiting_approval`) gets a highlighted row (`fti-current`), matching the reference mockup's "you are here" emphasis.
- `F-3` New container `FlowTimelineSidebar` (`components/FlowTimelineSidebar.tsx`) owns visibility (`isFlowModeRun(chatMode) && status === "running"`, both already-existing store helpers) and local expand/collapse state; it replaces `WorkflowStepRuntimePanel`, which is deleted.
- `F-4` `ChatWorkspace.tsx` renders `<FlowTimelineSidebar />` as its own flex child between `<main>` and the right sidebar's resizer/aside, so it can occupy width independently of the right sidebar's resizable width.
- `F-5` Reused existing badge classes (`pill-prov prov-*`, `ac-model`, `wsr-retry-badge`) for the per-step provider/model/retry chips (carried over from `BUG-155`) instead of inventing new ones, for visual consistency with `AgentsPanel`.

### Constraints

- No new npm dependency: icon glyphs are plain characters (✓ / ✕ / !) plus CSS-only circle/pulse styling, matching the rest of the app (no icon library present in `desktop-flowpilot`).
- The sidebar must fully unmount (not just visually hide) when Flow Mode isn't running, so it never reserves layout space outside an active flow run.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx` (new)
- `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx` (new)
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx` (layout wiring)
- `apps/desktop-flowpilot/src/styles.css` (`.flow-sidebar*`, `.flow-timeline*`, `.fti-*` rules)
- `apps/desktop-flowpilot/src/state/store.ts:1649-1661` (`isFlowModeRun`, `activeWorkflowStep` — reused, not duplicated)

## 1. Issue Summary

The Flow Mode step list needed a different visual (connected-circle timeline instead of flat cards) and a different location (its own collapsible sidebar between chat and the existing right sidebar, visible only while a flow is running) than what `BUG-153` shipped.

## 2. Parent Links

- impacted coding plan: `none`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot
- reproduction steps: n/a — this is a UI redesign request, not a defect reproduction.
- frequency: n/a

## 4. Expected vs Actual

- expected (post-fix): a collapsible sidebar between the chat column and the right sidebar, shown only during a running Flow Mode run, rendering a connected-circle stepper (5 icon states, current-step highlight) that collapses to an icon-only rail and expands to show a progress summary plus full per-step detail.
- actual (pre-fix): the step list was a flat card list permanently stacked in the existing right sidebar regardless of run state, with no connected-line visual and no collapse/expand affordance.

## 5. Impact

- users affected: anyone running Flow Mode.
- workflows affected: Flow Mode sidebar UI only.
- severity: low — cosmetic/UX, no functional regression to fix; this is an enhancement over `BUG-153`/`BUG-155`.

## 6. Root Cause

- hypothesis: n/a (feature/redesign request, not a defect).
- confirmed cause: n/a.
- evidence: n/a.

## 7. Fix Strategy

- `F-1` Add `FlowStepTimeline.tsx`: maps each `WorkflowStepRuntimeDTO.status` to one of `idle`/`running`/`done`/`approval`/`error`, renders a `<ol>` of connected circle+line items, and — when not `compact` — the step's name (`nodeId` fallback chain from `BUG-155`), status/rejection text, retry badge, and provider/model/agent/yolo meta.
- `F-2` Add `FlowTimelineSidebar.tsx`: gates rendering on `isFlowModeRun(chatMode) && status === "running"`; renders a `flow-sidebar-head` with a progress summary (`doneCount/total`, current step name via `activeWorkflowStep`) when expanded, and a toggle button; renders `FlowStepTimeline` in the body with `compact={!expanded}`.
- `F-3` Wire `<FlowTimelineSidebar />` into `ChatWorkspace.tsx` between `</main>` and the right sidebar block; remove `WorkflowStepRuntimePanel` from `right-sidebar-stack` and delete the now-superseded `WorkflowStepRuntimePanel.tsx`.
- `F-4` Add `.flow-sidebar*`/`.flow-timeline*`/`.fti-*` CSS: fixed collapsed (44px) vs expanded (260px) width with a `width` transition, circle icons colored via existing `--accent`/`--ok`/`--warn`/`--err` tokens, a pulsing ring on the running icon, and a connector line whose color reflects whether the step above it is done/running/idle.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean, no errors (including in the new/changed files).
- `V-2` `npx vitest run` in `apps/desktop-flowpilot` — could not run: all 5 test files fail identically with `ReferenceError: require is not defined in ES module scope` inside `vite-electron-renderer`'s `assert/strict.mjs` mock, including test files this change never touched (`navigatorHistory.test.ts`, `normalizeImage.test.ts`) — a pre-existing environment/tooling issue, not caused by this change.
- `V-3` Attempted a live preview (`npm run dev` via the existing `desktop-flowpilot` launch config): the app loads and reaches its "Bootstrapping desktop workspace…" screen with no console/server errors, but cannot proceed past that screen without a reachable local-runner backend and Supabase runtime config, neither of which is available in this environment — so the actual Flow Mode sidebar (collapsed rail, expand/collapse, icon states, current-step highlight) could not be visually confirmed. This should be checked in a real desktop-app session with a running local-runner and an active Flow Mode run before considering this fully verified.

## 9. Regression Guard

- tests: none added — no existing test harness renders `ChatWorkspace`/sidebar components (the vitest environment issue in `V-2` blocks adding one right now); recorded as a residual risk in `V-3`.
- alerts: none.
- audit checks: recorded in `change-audit/CA-192-flow-timeline-sidebar-redesign.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: none.
