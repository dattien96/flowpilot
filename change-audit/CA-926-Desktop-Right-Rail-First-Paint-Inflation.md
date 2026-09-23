# CA-926 — Desktop: right-rail panels inflate on first paint

## Summary

Right-rail sections (Mode tabs, Posture tabs, Worktree toggle, Agents) rendered
~1.5-2x oversized for the first seconds after app open, then snapped back once
the remaining panels mounted. User report: "UI có gì sai" — buttons Scan/Plan/
Code visibly inflated at startup, suspected CSS loading.

## Root cause

Layout, not stylesheet timing. `.right-sidebar-stack` is
`display: grid; height: 100%`. With the grid default `align-content: normal`
(behaves as `stretch`), auto-sized tracks absorb leftover container height.
During the first paint only a subset of rail sections have mounted (project
histories still "Loading...", Agents/Accounts not yet populated), so the few
mounted sections stretched vertically; each `.workflow-rail` section is itself
`display: grid`, so its inner rows stretched too, inflating `.tab-list` rows.

Measured in a live harness (real styles.css, real rail markup, Electron
renderer): `.tab` buttons computed at **174px height** while font-size/padding
were already correct (9px / 4px 8px) — proof the mechanism was grid stretch,
not FOUC.

## Fix

One declaration: `align-content: start` on `.right-sidebar-stack` — rows pack
at content height from the first frame regardless of how many sections have
mounted. Verified live: tab height 174px → 20px after the change (Vite HMR).

Regression guard: additive test in `styles.tokens.test.ts` asserts the stack
rule carries `align-content: start`.

## Files

- `apps/desktop-flowpilot/src/styles.css` — `.right-sidebar-stack` align-content.
- `apps/desktop-flowpilot/src/styles.tokens.test.ts` — new regression test.

## Verification

- `node --test --experimental-strip-types src/styles.tokens.test.ts` — 5/5 pass.
- Live Electron renderer probe: `.tab` height 174.23px → 20px post-fix.

# ---8<--- flowpilot:change-ledger
feature_key: desktop-ui-consistency
source_doc_id: user-report
change_type: bugfix
summary: pin right-sidebar-stack grid rows to content height to stop first-paint panel inflation
# --->8---
