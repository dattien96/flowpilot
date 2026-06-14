# BUG-047: Desktop Completed Tool Calls Hide From Timeline

## Metadata

- Document ID: `BUG-047`
- Title: `Desktop Completed Tool Calls Hide From Timeline`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-13`
- Last Updated: `2026-06-13`
- Parent Documents: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`
- Child Documents: `change-audit/CA-059-fix-desktop-thinking-row-persistence.md`
- Related Documents: `apps/desktop-flowpilot/src/components/Timeline.tsx`, `apps/desktop-flowpilot/src/styles.css`
- Replaces: `none`
- Tags: `desktop, ui, timeline, tool-calls, regression`

## AI Quick View

### Summary

- Desktop runs could finish with tool activity present in the event stream but not clearly visible in the final chat timeline.
- The renderer showed each tool row as a low-signal inline event, which made completed tool work easy to miss once the run settled.
- The fix grouped adjacent tool events into an explicit expandable summary row with stable status text.
- This is primarily a UI bug, caused by presentation behavior in the desktop timeline rather than provider/runtime execution.

### Current Ask

- Record the already-fixed desktop bug where tool-call UI was not visible enough after chat completion.

### Key Decisions

- `V-1` Classify this as a UI bug with local rendering logic impact, not a backend orchestration bug.
- `V-2` Preserve the existing event model and improve visibility in the timeline renderer instead of changing runner contracts.

### Constraints

- The fix must remain compatible with existing stored timeline items and live streaming events.
- Tool rendering changes must not break prompt, assistant, approval, or question timeline items.

### Open Questions

- None for this recorded bug.

### Source Refs

- User report: `ui of tool call will not show after the chat done`
- `apps/desktop-flowpilot/src/components/Timeline.tsx`
- `apps/desktop-flowpilot/src/styles.css`

## 1. Issue Summary

After a desktop chat run finished, completed tool activity was not clearly surfaced in the timeline. Users could miss what tools were called because those rows were easy to lose in the settled transcript.

## 2. Parent Links

- impacted coding plan: unknown
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: unknown

## 3. Environment and Reproduction

- environment: desktop FlowPilot timeline after a run with one or more tool events
- reproduction steps:
  1. Start a desktop run that emits tool events.
  2. Wait for the assistant response to finish.
  3. Inspect the final timeline.
- frequency: reproducible for tool-heavy runs in the previous renderer

## 4. Expected vs Actual

- expected: completed tool activity remains visible and easy to inspect after the run ends
- actual: tool activity is technically present but not surfaced clearly enough in the final timeline UI

## 5. Impact

- users affected: desktop users reviewing tool-heavy runs
- workflows affected: run inspection, debugging, auditability of agent actions
- severity: medium UX regression

## 6. Root Cause

- hypothesis: the desktop timeline rendered tool events with too little visual structure after completion
- confirmed cause: adjacent tool events were left as plain rows instead of a clear summarized UI group
- evidence: the fix lives entirely in `Timeline.tsx` and `styles.css`, with no runner-contract changes required

## 7. Fix Strategy

- `F-1` Group adjacent tool events into a `tool-group` summary row in the desktop timeline.
- `F-2` Show a compact completed/running/failed label for the group.
- `F-3` Allow expanding the group to inspect the underlying tool rows.

## 8. Validation

- `V-1` `npm run typecheck` in `apps/desktop-flowpilot` passed on 2026-06-13.
- `V-2` `npm run build` in `apps/desktop-flowpilot` passed on 2026-06-13.

## 9. Regression Guard

- tests: no dedicated renderer test harness exists yet in this package; regression is currently guarded by typecheck/build plus the timeline reducer tests added in `CA-059`
- alerts: none
- audit checks: `change-audit/CA-059-fix-desktop-thinking-row-persistence.md`

## 10. Follow-Up Document Updates

- upstream docs that must change: none
- notes left unchanged on purpose: this record classifies the issue as a UI bug because the provider events were already emitted; the failure was in timeline presentation
