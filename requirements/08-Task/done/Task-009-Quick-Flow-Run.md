# TASK-009: Quick Flow Run

## Goal
Provide a fast and accessible way for users to run their most frequently used workflows and steps directly from the Dashboard, saving them time from navigating through deep menus.

## Problem
Currently, users have to navigate to the Workflows or Steps page, find their desired flow/step, and then trigger it. This is tedious for workflows they run often.

## Expected Behavior
- Allow users to "Star" or "Favorite" specific workflows and steps from their respective listing pages.
- Add a "Quick Run" button to the Dashboard that opens a modal.
- The Quick Run modal displays all starred workflows and steps.
- Allow users to select a target project for the run from within the modal.
- Remember the last selected project for a favorite item so users don't have to select it every time if it doesn't change.
- Executing a step from the Quick Run modal automatically wraps it in an ad-hoc workflow and runs it.
- Executing immediately navigates the user to the live run page.

## Acceptance Criteria
- [x] Favorite (Star) toggle is present on Workflow cards in `/workflows`.
- [x] Favorite (Star) toggle is present on Step cards in `/workflow-steps`.
- [x] The Dashboard exposes a Quick Run favorites panel for starred workflows and steps.
- [x] The panel correctly lists all favorited items and separates workflows from steps.
- [x] Users can select a project for each favorite item and the last successful project choice is retained.
- [x] Users can start a workflow or step run from the Dashboard panel and are redirected to the active run page.

## Implementation Notes
- The dashboard exposes the quick-run experience as a dedicated favorites panel on the landing page rather than a single launcher button that opens a separate shell modal.
- Runs require an optional prompt to be entered in the run dialog; if left blank, the runtime falls back to a default dashboard prompt when the run starts.
- The selected project is persisted when a run is launched, which keeps the last successful project choice for that favorite item.
