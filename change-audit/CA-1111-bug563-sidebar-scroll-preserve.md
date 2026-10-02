# CA-1111 — BUG-563: right-sidebar scroll survives focus switches and SSE refreshes

## Why

Live run-69320: switching agent focus in the desktop app reset the right
sidebar (flow timeline + agents) scroll to the top. The scroller is the
shared `.right-sidebar-stack` container in `ChatWorkspace`, not
`AgentsPanel`: switching runs re-renders panels and shrinks content height
for a frame, the browser clamps `scrollTop` down, and the position is lost
when the content grows back.

## What changed

`apps/desktop-flowpilot/src/components/sidebarScrollPreserve.ts` (new):

- `sidebarScrollRestoreTop(remembered, current)` — pure decision: restore
  the remembered offset only when the browser clamped `scrollTop` BELOW it.
  No restore when nothing was remembered or the user has already scrolled
  at/past the remembered offset (a deliberate user scroll wins).

`apps/desktop-flowpilot/src/components/ChatWorkspace.tsx`:

- `rightStackRef` + `rightStackScrollTop` track the stack's scroll offset on
  every `onScroll`; a `useLayoutEffect` restores it when a rerender clamped
  the container below the remembered offset.
- No raw logs are surfaced; this is layout bookkeeping only.

## Invariant

Focus changes are reads, not layout resets — a user's scroll position in a
persistent panel is theirs until they move it.

## Tests

`sidebarScrollPreserve.test.ts`: restores when clamped below the remembered
offset; no restore when nothing was remembered; no restore when the user is
already at/past it. `npm run typecheck` clean.
