## Metadata

- Document ID: `BUG-252`
- Title: `Desktop Translate Popup Result Missing Scroll And Viewport-Aware Positioning For Long Content`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-07-07`
- Last Updated: `2026-07-07`
- Parent Documents: none (the translate-selection popup has no dedicated `SS`/`SD`/`CP` entry; it was added as part of the LibreTranslate integration tracked only at the change-audit level)
- Child Documents: none
- Related Documents: [CA-134: LibreTranslate Launch Detection And Port Routing](../../../change-audit/CA-134-libretranslate-launch-and-port.md), [CA-250: Cap Translate Popup Result Height And Add Scroll](../../../change-audit/CA-250-translate-popup-scroll-for-long-content.md)
- Replaces: none
- Tags: desktop, translate-popup, ui, css, regression

## AI Quick View

### Summary

- The "Translate to Vietnamese" result popover (`TranslatePopup.tsx`) rendered translated text with no height cap and positioned itself with a fixed `top`/`left` computed once from the selection rect, with no awareness of the viewport edges.
- First pass fixed only the missing scroll (`max-height` + `overflow-y` on `.translate-result-text`), but a fixed `240px` cap combined with a static "always below the selection" anchor still let the popover render off-screen when the selection was near the bottom of the window — visible to the user as "the popup only shows once, then doesn't show again, is too small, and can't be dragged into view."
- The full fix replaces the static anchor with a `useLayoutEffect`-driven placement: flip above the selection when there isn't room below, clamp horizontally within the viewport, and size the scrollable text region to whatever space is actually available (instead of a hardcoded `240px`) — so the popover is always fully visible without needing to be dragged.

### Current Ask

- "fix UI translate. Cần scroll khi content dài" — add scrolling to the translate popup when the translated content is long.
- Follow-up after the first fix: "giờ thì bấm modal không show luôn" / "bấm dịch lại lần 2 không show. Modal height cũng quá nhỏ + k drag được lúc nào cũng ở bottom" — clicking translate a second time doesn't show the popup, the box is too small, and it can't be dragged out of the bottom of the screen.

### Key Decisions

- `V-1` The translated-text block must cap its height and become internally scrollable once content exceeds that height, instead of growing the popover unbounded.
- `V-2` The popover header (language pair, copy, dismiss) must stay fixed and visible regardless of how long the translated text is.
- `V-3` The popover must never render outside the viewport: if there isn't enough room below the selection, it must flip above; horizontal position must clamp within the window edges; the scrollable text region's max height must be computed from actual remaining space, not a fixed constant.

### Constraints

- Do not change the translate request/response flow (`runnerFetch`, `/translate` endpoint) — this is a display-only fix.
- Do not regress the existing compact appearance for short translations.
- Do not add manual drag-to-reposition — automatic viewport-aware placement removes the need for it.

### Open Questions

- None.

### Source Refs

- `apps/desktop-flowpilot/src/components/TranslatePopup.tsx`
- `apps/desktop-flowpilot/src/styles.css` (`.translate-popover`, `.translate-result`, `.translate-result-text`)

## 1. Issue Summary

When a user selects text in the chat timeline and translates it, the result popover shows the translated text in a `.translate-result-text` block. Two related defects were found in the same investigation:

1. The text block had no `max-height`/`overflow` styling, so long translations grew the popover past the visible viewport with no scrollbar.
2. Even after capping the text block's height at a fixed `240px`, the popover's *position* was still computed once as `{ left: rect.left + rect.width/2, top: rect.bottom + 8 }` with no viewport-bounds check. Selections made near the bottom of the window (a common case — translating the most recent message near the composer) placed the popover partly or fully below the visible window, which the user experienced as "the popup doesn't show" on repeat attempts, plus "too small" and "stuck at the bottom with no way to drag it into view."

## 2. Parent Links

- impacted coding plan: none — no `CP` document governs the translate-selection popover specifically
- impacted tech design: none
- impacted system spec: none

## 3. Environment and Reproduction

- environment: desktop-flowpilot app (`apps/desktop-flowpilot`), chat Timeline view
- reproduction steps:
  1. Open a chat run with a long assistant/user message near the bottom of the visible Timeline (close to the composer).
  2. Select a long span of text (a paragraph or more).
  3. Click "Translate to Vietnamese" in the popup chip.
  4. Observe that the result box is clipped by, or renders past, the bottom of the window, with no scrollbar and no way to reposition it.
- frequency: consistent whenever the selection's bounding rect is close enough to the bottom (or top, for very tall content) of the viewport that `rect.bottom + 8 + popoverHeight` exceeds `window.innerHeight`

## 4. Expected vs Actual

- expected: the popover always renders fully inside the viewport — flipping above the selection when there isn't room below — and the translated-text area scrolls internally using whatever height is actually available, rather than a fixed constant
- actual (before this fix): `.translate-result-text` had no `max-height`/`overflow-y` at all; after the first partial fix, it had a static `240px` cap but the popover's screen position was still unaware of viewport edges, so it could render off-screen regardless of the text cap

## 5. Impact

- users affected: any desktop-flowpilot user translating a selection near the top/bottom edge of the visible Timeline, or translating long text
- workflows affected: inline translate-on-selection feature in the chat Timeline
- severity: low-medium — feature is still usable for selections near the vertical center of the screen, but unusable (perceived as "not showing") near the viewport edges

## 6. Root Cause

- hypothesis: the translated-text block had no height constraint
- confirmed cause (two parts):
  1. `.translate-result-text` in `apps/desktop-flowpilot/src/styles.css` originally set only padding/font/line-height/`white-space: pre-wrap`, with no `max-height` or `overflow-y`.
  2. `TranslatePopup.tsx` positioned the popover with a single fixed `{ x, y }` anchor derived once from `range.getBoundingClientRect()`, applied via `position: fixed; top; left; transform: translateX(-50%)`, with no logic to detect or correct for the popover extending past `window.innerHeight`/`innerWidth`.
- evidence: read `TranslatePopup.tsx` (renders `result.translatedText` directly inside `.translate-result-text`, and set `style={{ left: anchor.x, top: anchor.y }}` unconditionally) and the corresponding CSS block at `apps/desktop-flowpilot/src/styles.css:6179-6185` prior to the fix; reproduced the off-screen case in a browser preview by simulating a selection anchor at `window.innerHeight - 20` — the popover's computed bounding rect extended well past the viewport bottom

## 7. Fix Strategy

- `F-1` Add `max-height`, `overflow-y: auto`, and `overscroll-behavior: contain` to `.translate-result-text` in `apps/desktop-flowpilot/src/styles.css` (kept as the pre-measurement CSS fallback; the real cap is now computed in JS — see `F-2`).
- `F-2` In `TranslatePopup.tsx`, replace the single-shot `{x, y}` anchor with a `useLayoutEffect` that runs whenever the anchor or popover content (`pending`/`result`/`loading`/`error`) changes:
  - measure the rendered popover's `offsetWidth`/`offsetHeight`;
  - clamp horizontal position (`left`) within `[VIEWPORT_MARGIN, innerWidth - width - VIEWPORT_MARGIN]`;
  - decide whether the popover fits below the selection (`spaceBelow`) or must flip above (`spaceAbove`), and anchor to a fixed viewport edge (`anchor.bottom + 8` when below, `VIEWPORT_MARGIN` when above) rather than a height-dependent offset, so the far edge is guaranteed to land within the available space regardless of the box's actual rendered height;
  - compute `textMaxHeight` for `.translate-result-text` from the real available space minus the measured header height (via a new `headerRef`), instead of the fixed `240px`.
- `F-3` Remove the unconditional `transform: translateX(-50%)` from the base `.translate-popover` CSS rule (it now only applies as an inline fallback style before the first layout measurement, since the JS-computed `left` already accounts for the box's own width).

## 8. Validation

- `V-1` Verified via browser preview by injecting a `.translate-result` block with ~60 lines of Vietnamese placeholder text using the real CSS classes: `overflowY: "auto"`, and `scrollHeight` (1188px) exceeds `clientHeight` in every scenario tested, confirming the block is always scrollable regardless of the computed max-height.
- `V-2` Manual code review confirms the popover header (`.translate-result-header`) is outside the scrollable `.translate-result-text` block, so language pair / copy / dismiss controls remain visible while the text area scrolls.
- `V-3` Verified the viewport-aware placement algorithm (extracted to a standalone JS harness matching the component logic exactly) against 6 scenarios in a real browser viewport: long text with selection near the bottom (the reported bug), near the top, dead center, short text near the bottom, near the left edge, and near the right edge. All 6 report the popover's final bounding rect fully within `[0, innerWidth] x [0, innerHeight]` (`fitsViewport: true` in every case). The bottom-edge scenario in particular now flips the popover above the selection and caps its text region to the actual available space (verified before this fix it computed a `top` that placed the box hundreds of pixels below the viewport).
- `V-4` `npm run typecheck` (`tsc --noEmit`) in `apps/desktop-flowpilot` passes with no errors after the `TranslatePopup.tsx` changes.
- `V-5` No console errors in the browser preview during any of the above checks.
- Not run: full desktop-flowpilot test suite / manual end-to-end reproduction through the live app UI — the app's bootstrap flow requires a running local-runner backend, which was unavailable in this environment, so verification was done via a standalone browser-preview harness that exercises the exact CSS classes and position/sizing algorithm rather than the mounted `Timeline`/`TranslatePopup` React tree end-to-end.

## 9. Regression Guard

- tests: none added — this is a UI display/positioning fix with no existing automated coverage for `TranslatePopup`; verified manually via the browser-preview harness described in `V-1`-`V-3`
- alerts: none
- audit checks: `git diff` limited to `apps/desktop-flowpilot/src/components/TranslatePopup.tsx` (positioning/sizing logic) and `apps/desktop-flowpilot/src/styles.css` (`.translate-popover`, `.translate-result-text`); no changes to the `/translate` request path or other components

## 10. Follow-Up Document Updates

- upstream docs that must change: none — no business behavior, acceptance criteria, or interface contract changed; this is a display/positioning fix
- notes left unchanged on purpose: no `SS`/`SD`/`CP` exists for this popup; not creating one since the fix is a scoped UI regression, not a new design decision. Manual end-to-end verification through the mounted app (with the local-runner backend running) is recommended as a follow-up since this environment could not boot the full desktop app.

## 11. Follow-Up Addendum (2026-07-07)

- A later follow-up bug report clarified that the popup must remain anchored to the selected source text, prefer showing below that selection by default, flip above when the selection is too low in the viewport, allow the translated result card to be dragged, and dismiss only on outside pointer interaction.
- `TranslatePopup.tsx` now keeps the viewport-aware placement logic from the first fix, adds bounded header dragging via pointer events, and changes dismissal from the old `mousedown` behavior to an outside-only `pointerdown` guard that ignores events originating inside `popoverRef`.
- Validation for this addendum: `npm run typecheck` in `apps/desktop-flowpilot` passed after the interaction changes, and the code path was manually reviewed for four cases: below-anchor default, above-anchor flip, inside-click no-dismiss, and outside-click dismiss.
