# CA-1106 — BUG-558: back-to-main scroll restore survives regroup + window shift

## What
`Timeline.tsx`'s Task-433 scroll-anchor restore now (a) tries regrouped
`approval-group-*`/`question-group-*` forms of the captured id, (b) expands
the visible prompt window when the anchored item still exists but is sliced
out by prompts that arrived while the child view was focused, and (c) falls
back to the bottom when the anchored id vanished entirely — instead of
leaving a pending anchor that never resolves and the restored view clamped
at the top.

## Why
User report: entering a sub-agent detail then "← Back to main agent" always
landed scrolled up. The Task-433 anchor silently stayed pending whenever the
captured id could not materialize — grouping re-ids lone approvals/questions
when siblings append, and new prompts shrink the rendered slice on a busy
run.

## Guarantees kept
- Successful anchor resolution is byte-identical to before (same offset math).
- Pure helpers (`timelineScrollAnchor.ts`) keep the resolution testable;
  rendering behavior unchanged when the anchor resolves normally.
- Bottom fallback only fires when the item is absent from the restored
  timeline — a pending anchor that can still materialize keeps waiting.

## Tests
`timelineScrollAnchor.test.ts` (5 cases, all green).
