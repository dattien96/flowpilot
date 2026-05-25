# CA-009 Theme Switcher Icon-Only Update

## Scope

Updated the `ThemeSwitcher` component to be icon-only to improve the sidebar appearance, preventing text labels from truncating with ellipsis when the sidebar is compact.

## Completed

- Updated [theme-switcher.tsx](file:///c:/working/flowpilot/apps/admin-web/src/features/theme/components/theme-switcher.tsx):
  - Simplified layout to only show icons (`Sun`, `Moon`, `Laptop`).
  - Removed text labels entirely.
  - Made the icon grid container full-width and centered the icons inside.
  - Ensured correct hover/active styles are maintained for visual clarity.
  - Retained tooltips (`title` attribute) so that hover displays descriptive names ("Switch to Light theme", etc.).

## Verification

- Ran unit tests in `apps/admin-web` via `npm run test` (all 131 tests passed successfully).
