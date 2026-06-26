# CA-063: Fix Desktop Settings Nav Order And Runner Status Cue

## Scope

- Corrected the desktop settings sidebar ordering so `Supabase` appears below the main settings sections and directly above `Runner`.
- Added a small inline status dot beside the `Runner` nav item so reachability is visible in the sidebar, not only in the header status widget.
- Kept the change confined to the desktop settings shell and styles; no routing or backend behavior changed.

## Completed

- Introduced an explicit sidebar order in `SettingsShell` instead of relying on the default section list order.
- Rendered the `Runner` nav label with an adjacent status dot driven by `runtimeStatus.runnerReachable`.
- Added a tiny layout helper class in `styles.css` so the label and dot align cleanly in the nav item.
- Recorded the issue and fix path in `BUG-053`.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot`
- Targeted source review of `SettingsShell.tsx` and `styles.css` after the patch

## Residual Notes

- No dedicated UI test currently covers sidebar ordering or the inline runner cue.
- The broader R3 phase-1 parity checklist still contains other desktop-vs-web gaps beyond this localized fix.

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: BUG-053
change_type: fix
summary: Fix Desktop Settings Nav Order And Runner Status Cue
# --->8---
