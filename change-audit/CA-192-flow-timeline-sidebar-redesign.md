# CA-192: Flow Timeline Sidebar Redesign

## Summary

Implemented `BUG-156`: replaced the flat step-card list (`WorkflowStepRuntimePanel`, shipped in `BUG-153`) with a vertical connected-circle step timeline, and moved it out of the shared right sidebar into its own dedicated collapsible sidebar that only renders while a Flow Mode run is active.

## What Changed

- Added `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx`: presentational connected-circle stepper with 5 visual states (idle/running/done/approval/error) derived from `WorkflowStepRuntimeStatus`; `compact` prop toggles between icon-only rail and full title/description/meta view.
- Added `apps/desktop-flowpilot/src/components/FlowTimelineSidebar.tsx`: new collapsible sidebar container, visible only when `isFlowModeRun(chatMode) && status === "running"`; shows a progress summary + expand/collapse toggle.
- `apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`: renders `<FlowTimelineSidebar />` between the chat `<main>` and the existing right sidebar; removed `WorkflowStepRuntimePanel` from `right-sidebar-stack`.
- Deleted `apps/desktop-flowpilot/src/components/WorkflowStepRuntimePanel.tsx` (superseded).
- `apps/desktop-flowpilot/src/styles.css`: added `.flow-sidebar*` (collapsed/expanded width + transition), `.flow-timeline*`/`.fti-*` (circle icons, pulse animation, connector line) rules.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `npx vitest run` — blocked by a pre-existing, unrelated environment issue (`vite-electron-renderer`'s `assert/strict` mock breaks under ESM for every test file, not just ones touched here).
- Live preview reaches the app's pre-backend "Bootstrapping desktop workspace…" screen with no errors, but couldn't proceed far enough (no local-runner/Supabase available in this environment) to visually confirm the sidebar. Flagged as unverified in `BUG-156` (`V-3`).

## Notes

- `RuntimeWorkflowStep`/DTO fields are unchanged from `BUG-155` — this is a pure presentation/layout change on top of that data.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-156
change_type: feature
summary: Replace the flat Flow Mode step-card list with a connected-circle timeline in a new dedicated collapsible sidebar shown only while a flow is running
# --->8---
