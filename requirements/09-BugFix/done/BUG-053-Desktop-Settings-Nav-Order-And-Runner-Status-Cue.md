# BUG-053: Desktop Settings Nav Order And Runner Status Cue

## Metadata

- Document ID: `BUG-053`
- Title: `Desktop Settings Nav Order And Runner Status Cue`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`, `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`, `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Checklist.md`
- Child Documents: `none`
- Related Documents: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`, `apps/desktop-flowpilot/src/components/SettingsShell.tsx`, `apps/desktop-flowpilot/src/styles.css`, `change-audit/CA-063-fix-desktop-settings-nav-order-and-runner-status-cue.md`
- Replaces: `none`
- Tags: `desktop-flowpilot, settings, navigation, ui, regression`

## AI Quick View

### Summary

- The desktop settings sidebar rendered `Supabase` before the other settings sections, which did not match the intended order captured in the R3 phase-1 review.
- The `Runner` tab had no inline status cue, so the menu did not surface runner reachability at a glance.
- Both issues were fixed in the desktop settings shell with a localized UI-only change.

### Current Ask

- Record the two desktop settings navigation issues from the R3 phase-1 review capture as a formal bug fix entry.

### Key Decisions

- `V-1` Keep the fix local to `SettingsShell` and the related sidebar styles.
- `V-2` Render `Supabase` below the main settings sections and directly above `Runner`.
- `V-3` Show a small status dot next to `Runner` using the existing `runtimeStatus.runnerReachable` value.

### Constraints

- Do not change settings routing, section visibility rules, or backend data flow.
- Keep the fix consistent with the existing desktop shell style system.

### Open Questions

- None.

### Source Refs

- `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Review-Capture-Issues.md`
- `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
- `apps/desktop-flowpilot/src/styles.css`

## 1. Issue Summary

The desktop settings sidebar did not match the review-captured ordering and affordance expectations. `Supabase` appeared before the rest of the settings sections, and `Runner` was just a text label without an inline status indicator.

## 2. Parent Links

- impacted coding plan: `requirements/10-Refactor/New-System/04-01-Phase1-Desktop-Mock-MVP.md`
- impacted tech design: `requirements/10-Refactor/New-System/03-Solution-And-System-Design.md`
- impacted system spec: `requirements/10-Refactor/Migrate-Web-To-Desktop/R3-Phase1-Checklist.md`

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` settings shell in the Electron desktop app
- reproduction steps:
  1. Open the desktop app settings view.
  2. Inspect the left navigation order.
  3. Compare the `Runner` item with the other tabs.
- frequency: always before the fix

## 4. Expected vs Actual

- expected: `Supabase` should sit below the main settings sections and above `Runner`, and `Runner` should show an immediate reachability cue in the menu.
- actual: `Supabase` rendered first in the nav list, and `Runner` had no inline status dot.

## 5. Impact

- users affected: desktop operators using the settings shell
- workflows affected: settings navigation, quick runner-status scanning
- severity: low, because the bug was UI parity and orientation related rather than a data-loss or execution issue

## 6. Root Cause

- hypothesis: the sidebar order and runner affordance were left as generic section rendering rather than matching the review-captured desktop parity expectations.
- confirmed cause: `SettingsShell` used the default section array order directly, which placed `supabase` first, and the `Runner` nav item had no inline status indicator in the menu.
- evidence:
  - `SettingsShell` now uses an explicit navigation order with `supabase` and `runner` at the end.
  - `RunnerStatusIndicator` already existed for the header, so the sidebar cue could reuse the shared `status-dot` styling and `runtimeStatus.runnerReachable`.

## 7. Fix Strategy

- `F-1` Reorder the settings navigation list in `SettingsShell` so `Supabase` renders just above `Runner`.
- `F-2` Add a small inline status dot to the `Runner` nav item using the existing runner reachability flag.

## 8. Validation

- `V-1` `npm run typecheck` passes in `apps/desktop-flowpilot`.
- `V-2` Source review of `SettingsShell` confirms the new nav order and the `Runner` status cue are both wired from the same `runtimeStatus.runnerReachable` source.

## 9. Regression Guard

- tests: none added for this localized sidebar ordering change
- alerts: none
- audit checks: keep the fix limited to the desktop settings shell unless a broader navigation contract is later documented upstream

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`
- notes left unchanged on purpose:
  - the review capture file remains the source of the original issue list.
  - this bug record documents the desktop-only parity correction, not a broader route or backend contract change.
