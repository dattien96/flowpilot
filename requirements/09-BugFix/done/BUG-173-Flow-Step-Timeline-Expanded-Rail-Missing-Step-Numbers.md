# BUG-173: Flow Step Timeline Expanded Rail Missing Step Numbers

## Metadata

- Document ID: `BUG-173`
- Title: `Flow Step Timeline Expanded Rail Missing Step Numbers`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-02`
- Last Updated: `2026-07-02`
- Parent Documents: `requirements/09-BugFix/done/BUG-156-Flow-Timeline-Moved-To-Dedicated-Collapsible-Sidebar.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-158-Flow-Timeline-Missing-Step-Name-Model-Agent-Yolo.md`, `requirements/09-BugFix/done/BUG-168-Flow-Step-Timeline-Sidebar-Hidden-During-Approval-Or-Question.md`
- Replaces: `none`
- Tags: `agent-flow-engine, ui, desktop, flow-mode, cosmetic`

## AI Quick View

### Summary

- In Flow Mode's step-timeline sidebar, the **collapsed** rail numbers each step 1-2-3-4, but the **expanded** rail shows blank circles for pending/running steps.
- Root cause: `FlowStepTimeline` rendered `compact ? index + 1 : STATE_GLYPH[state]`, and `STATE_GLYPH` is an empty string for both `idle` and `running` — so expanded pending/running steps had no glyph at all.
- The two rails should read consistently; the step index is the more useful marker than the sparse glyph set.

### Current Ask

- Show the step number (1-2-3-4) in the expanded rail too, matching the collapsed rail, while keeping status legible.

### Key Decisions

- `V-1` Both rails now render the step index for non-terminal states (idle/running/pending) and keep the `✓`/`✕` completion/error glyphs for `done`/`error`. Status stays conveyed by the existing `fti-{state}` color classes and the current-step highlight.

### Constraints

- Pure display-logic change in one component (`FlowStepTimeline.tsx`); no change to step-runtime data, ordering, or the sidebar visibility predicate (BUG-168).

### Open Questions

- Whether `done` steps should show `✓` (kept) or the plain number for a literal "1-2-3-4 everywhere" reading. Chosen to keep `✓`/`✕` as an at-a-glance completion/error cue layered on top of the numbering; trivially adjustable if pure numbers are preferred.

### Source Refs

- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx:28` (`STATE_GLYPH` — empty for idle/running)
- `apps/desktop-flowpilot/src/components/FlowStepTimeline.tsx:90` (icon render expression — the fix site)

## 1. Issue Summary

In the Flow Mode step-timeline sidebar, the collapsed (icon-rail) view numbers steps 1-2-3-4, but the expanded view shows empty circles for steps that are pending or running, so the two views look inconsistent and the expanded rail loses the step ordering at a glance.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md`
- impacted tech design: `none`
- impacted system spec: `none`

## 3. Environment and Reproduction

- environment: desktop-flowpilot, a Flow Mode run with the step-timeline sidebar expanded.
- reproduction steps:
  1. Start a Flow Mode run so the step-timeline sidebar shows.
  2. Expand the sidebar.
  3. Observe pending/running steps render as blank circles (no number), unlike the collapsed rail which shows 1-2-3-4.
- frequency: deterministic.

## 4. Expected vs Actual

- expected: expanded rail shows 1-2-3-4 like the collapsed rail.
- actual: expanded rail shows empty circles for pending/running steps.

## 5. Impact

- users affected: anyone using Flow Mode with the sidebar expanded.
- workflows affected: Flow Mode UI only; cosmetic.
- severity: low — purely visual consistency.

## 6. Root Cause

- confirmed cause: `FlowStepTimeline.tsx:90` rendered `{compact ? index + 1 : STATE_GLYPH[state]}`; `STATE_GLYPH` (`:28`) maps `idle` and `running` to `""`, so expanded non-terminal steps rendered an empty icon.
- evidence: `STATE_GLYPH = { idle: "", running: "", done: "✓", approval: "!", error: "✕" }`.

## 7. Fix Strategy

- `F-1` Changed the icon expression to render `index + 1` for all non-terminal states in both modes and keep `STATE_GLYPH` only for `done` (`✓`) and `error` (`✕`): `{state === "done" || state === "error" ? STATE_GLYPH[state] : index + 1}` (`FlowStepTimeline.tsx:90`). Status remains conveyed by the `fti-{state}` classes and the current-step highlight.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` — clean.
- `V-2` Not executed: a live render of the sidebar in a Flow Mode run — the timeline only renders inside an active flow run, which needs the Go runner + a provider account (unavailable in this environment). The change is a one-line display expression mirroring the existing collapsed-rail behavior; low risk.

## 9. Regression Guard

- tests: none (no render-test harness for this component, consistent with BUG-156/158/168).
- alerts: none.
- audit checks: recorded in `change-audit/CA-210-flow-timeline-expanded-rail-step-numbers.md`.

## 10. Follow-Up Document Updates

- upstream docs that must change: none.
- notes left unchanged on purpose: the `approval` glyph (`!`) is no longer shown in the icon (approval steps show their number); the `fti-approval` color class still signals the state, and the chat-column approval card remains the primary approval surface.
