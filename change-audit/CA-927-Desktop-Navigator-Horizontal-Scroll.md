# CA-927 — Desktop: navigator horizontal scrollbar at narrow widths

## Summary

Manual CP-83 M-7 verification: shrinking the window produced a horizontal
scrollbar at the bottom of the left project/history navigator. Vertical scroll
is intended; horizontal must never appear.

## Root cause

Measured live via CDP on the real app (960px window): `.navigator` had
`scrollWidth 240 > clientWidth 229` (+11px → +19px inside the rail section).
Bisect by hiding elements isolated `.project-group-head`: the longest project
name ("Meal Suggestion Android App 1") is `white-space: nowrap`, so its
min-content width propagates up the chain
`.project-group-name → .project-group-toggle → .project-group-head →
.project-group → .project-groups` and forces the `.project-rail-section`
implicit `auto` grid column ~19px wider than the container.

Two compounding details:
- `.navigator` declared `overflow-y: auto` only — per spec the other axis then
  computes to `auto`, so the overflow materialised as an h-scrollbar.
- No `min-width: 0` anywhere on the chain, so flex/grid children could not
  shrink below the nowrap text's min-content size.

## Fix

- `.project-groups` gets `min-width: 0` — the actual circuit-breaker; measured
  `scrollWidth` 240 → 229 = `clientWidth`.
- `.navigator` gets `overflow-x: hidden` — explicit one-axis intent and a
  defensive guard for any future overwide child.

## Verification

Live CDP probe after the change — `scrollWidth == clientWidth` at window
widths 960 / 800 / 700 / 640px (`navOX: hidden`, `groupsMinW: 0px`).
Additive regression test in `styles.tokens.test.ts` asserts both
declarations.

## Files

- `apps/desktop-flowpilot/src/styles.css`
- `apps/desktop-flowpilot/src/styles.tokens.test.ts`

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: CP-83
change_type: bugfix
summary: navigator h-scrollbar — min-width:0 on .project-groups + overflow-x:hidden on .navigator
# --->8---
