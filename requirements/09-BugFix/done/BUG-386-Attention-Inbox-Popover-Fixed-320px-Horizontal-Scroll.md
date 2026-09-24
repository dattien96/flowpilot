# BUG-386: Attention inbox popover capped at 320px and scrolls horizontally

- Document ID: `BUG-386`
- Status: `done`
- Severity: medium — needs-attention popover unusable overview; title text clips without ellipsis
- Area: `apps/desktop-flowpilot` `AttentionInbox` popover (`styles.css`)
- Feature key: `attention-queue` / `project-nav`

## Symptom

The header "Needs attention" popover renders at a fixed ~320px and shows a
horizontal scrollbar. Row titles clip at the popover edge instead of
ellipsizing, so the title + project + waiting-time + watch affordance are cut
off. Screenshot: `image/image.png` (2026-09-24 report).

## Expected vs Actual

- expected: popover uses available window width (expands when the window is
  wide, only shrinks when the window is dragged thin); no horizontal scroll —
  every row ellipsizes.
- actual: `width: min(320px, 90vw)` hard cap; rows overflow the 320px track
  and the popover scrolls horizontally.

## Root cause

- `.attention-inbox-pop` is capped at `min(320px, 90vw)`.
- `.attention-inbox-list` is `display: grid`; its items
  (`.attention-inbox-li`) keep `min-width: auto`, so the implicit column sizes
  to the widest row's min-content (~350px+: expand caret + checkbox + kind
  chip + longest title word + project name + waiting label + watch button).
  The grid track overflows the 320px popover, and `overflow-y: auto` promotes
  `overflow-x` to `auto` → horizontal scrollbar.
- Because the `<li>` is sized by min-content, the inner
  `.attention-inbox-item` flex row never gets constrained — the title never
  shrinks, so `text-overflow: ellipsis` never engages and text is hard-clipped
  by the popover edge.
- Side issue: `.attention-inbox-head` is a plain block, so
  `.inbox-batch-approve`'s `margin-left: auto` never applies — the
  "Approve all" button is not right-aligned.

## Fix

- `F-1` `.attention-inbox-pop`: widen to `min(560px, calc(100vw - 2 * var(--space-4)))` —
  fills available width on wide windows, shrinks responsively on thin windows;
  add `overflow-x: hidden` as the residual-overflow clip (same contract as
  `.navigator`).
- `F-2` `.attention-inbox-li`, `.attention-inbox-item`: `min-width: 0` so the
  grid track can shrink below min-content and the title ellipsis engages.
- `F-3` `.attention-inbox-head`: `display: flex; align-items: center;
  justify-content: space-between` so the batch-approve button right-aligns.

## Validation

- `V-1` Popover renders wider than 320px on a >= 900px window; rows ellipsize,
  no horizontal scrollbar. — implemented via `min(560px, 100vw - 2*space-4)`;
  manual drag check is an operator pass.
- `V-2` At a dragged-thin window the popover shrinks with `100vw` bounds —
  still no horizontal scroll (`overflow-x: hidden` is the residual clip).
- `V-3` `tsc -p tsconfig.phase1-tests.json` clean (exit 0); new render/store
  tests green. Pre-existing suite failures unrelated to this fix are logged in
  `CA-964`.
