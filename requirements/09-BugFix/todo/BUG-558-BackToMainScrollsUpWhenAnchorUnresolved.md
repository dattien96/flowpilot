# BUG-558 — "Back to main agent" leaves the timeline scrolled up

## Status
RESOLVED — see `timelineScrollAnchor.ts` + the restore effect in
`Timeline.tsx`.

## Report
Live user report (2026-10-02): entering a sub-agent run detail then clicking
"← Back to main agent" lands scrolled up instead of restoring the previous
reading position.

## Root cause
Task-433's scroll anchor restore waits for the anchored `[data-item-id]` node
to exist in the DOM and silently stays pending forever when it never does.
Two live ways the anchor becomes unresolvable while the child view is
focused:

1. **Regrouped id** — a lone approval/question item captured as `approval-N`
   re-ids to `approval-group-approval-N` when a sibling appends during the
   away window (buildTimelineGroups). The pending anchor queries the old id
   forever.
2. **Window shift** — new prompts appended to the main run while viewing the
   child shrink the rendered slice (`visiblePromptCount` pages by prompt
   count); the anchored item still exists in `timeline` but is above the
   rendered window, so the node never materializes.

Meanwhile the shorter child timeline clamped `scrollTop`, so restore landed
near the top — perceived as "scrolled up".

## Fix
`Timeline.tsx` restore effect now resolves through
`scrollAnchorCandidates` (raw + `approval-group-*`/`question-group-*` forms)
and, when no node is rendered, uses `scrollAnchorRevealPrompts` to expand
`visiblePromptCount` so the anchored item re-renders. If the anchored id no
longer exists in the timeline at all, the pending anchor is cleared and the
view falls back to the bottom instead of staying clamped at the top.

## Tests
`timelineScrollAnchor.test.ts` — candidate variants, reveal-prompt math,
grouped-id demotion, vanished-id → bottom fallback.
