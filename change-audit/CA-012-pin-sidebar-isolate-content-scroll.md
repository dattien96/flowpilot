# CA-012 Pin Sidebar and Isolate Content Scroll

## Scope

Improved the application grid layout to pin the sidebar and isolate scrollability. This ensures that the left sidebar remains static and visible while the right main content pane scroll behaves independently on large screens.

## Completed

- Updated [app-shell.tsx](file:///c:/working/flowpilot/apps/admin-web/src/components/layout/app-shell.tsx):
  - Abstracted layout tailwind classes into `APP_SHELL_LAYOUT_CLASSES`.
  - Configured `lg:overflow-hidden` on the outer layout to pin overall viewport.
  - Set `lg:h-full lg:min-h-0` on grid container.
  - Implemented `lg:sticky lg:top-4 lg:h-[calc(100vh-2rem)] lg:self-start lg:overflow-y-auto` on the sidebar (`aside`) element.
  - Configured `lg:h-[calc(100vh-2rem)] lg:min-h-0 lg:overflow-y-auto` on the `main` content pane to isolate its scrolling.
- Updated [app-shell.test.tsx](file:///c:/working/flowpilot/apps/admin-web/src/components/layout/app-shell.test.tsx):
  - Exported `APP_SHELL_LAYOUT_CLASSES` to verify grid layout constraints.
  - Added unit test asserting scroll-isolating class configurations on outer layout, grid, sidebar, and main components.

## Verification

- Ran unit tests in `apps/admin-web` via `npm run test` (all tests passed successfully, including the newly added layout assertion).

# ---8<--- flowpilot:change-ledger
feature_key: project-nav
source_doc_id: CA-012
change_type: feature
summary: Pin Sidebar and Isolate Content Scroll
# --->8---
