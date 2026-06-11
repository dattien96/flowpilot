# BUG-037: Stepper Indicator Does Not Show Error State on Stale or Failed Config

## Metadata

- Document ID: `BUG-037`
- Title: `Stepper Indicator Does Not Show Error State on Stale or Failed Config`
- Phase: `bugfix`
- Status: `done`
- Owner: `Antigravity`
- Reviewers: `User`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `[CP-28-Google-Cloud-Config-With-Ui-Auto.md](file:///C:/working/flowpilot/requirements/07-Coding-Plan/done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)`, `[CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md](file:///C:/working/flowpilot/requirements/07-Coding-Plan/priority/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md)`
- Child Documents: None
- Related Documents: `[CA-045-stepper-indicator-error-states.md](file:///C:/working/flowpilot/change-audit/CA-045-stepper-indicator-error-states.md)`
- Replaces: None
- Tags: `ui, settings, stepper, bug`

## AI Quick View

### Summary

- The 7-step Google Drive setup page stepper indicator only displayed success (configured/online), active, or neutral states.
- If a step was in a failed, reconnect required, needs auth, or stale config state, it rendered as a neutral (inactive) gray or active outline state rather than highlighting the error.
- Step 7 ("Proxy MCP setup") specifically tracked `status?.mcp.status` instead of the provider configurations status, causing it to display as configured/online even when provider configs were stale or missing input.

### Current Ask

- Display warning (needs input, reconnect required, needs auth, stale config) and error (failed) states in the step indicator.
- Map Step 7's status to the provider configurations status (`providerSetupStatus`).
- Add a new validation test.

### Key Decisions

- `V-1` Verify that Lucide's `AlertCircle` (circle-alert) and `TriangleAlert` are rendered in JSDOM when stepper elements encounter error and warning states.

### Constraints

- UI styling must match the design system, utilizing `danger` (red) and `warning` (orange) palettes for errors and warnings.

### Open Questions

- None.

### Source Refs

- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.test.tsx`

## 1. Issue Summary

The stepper component on the Google Drive setup page did not show any error or warning state. Steps in warning or error states (e.g. failed, reconnect_required, needs_auth, config_stale, or needs_input) were rendered with either neutral (gray) or active outline styles, making it hard for users to quickly diagnose which steps are blockages. Additionally, Step 7 tracked the proxy MCP status rather than the actual provider configuration status, masking setup issues.

## 2. Parent Links

- Impacted coding plan: `[CP-28-Google-Cloud-Config-With-Ui-Auto.md](file:///C:/working/flowpilot/requirements/07-Coding-Plan/done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)`, `[CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md](file:///C:/working/flowpilot/requirements/07-Coding-Plan/priority/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md)`

## 3. Environment and Reproduction

- Environment: Local web environment / Vitest runner.
- Reproduction steps: Navigate to `/settings/google-drive-setup` when one of the steps is not configured or fails. Observe that the step circle remains gray/inactive or active outline, rather than showing a distinct red/orange indicator.
- Frequency: 100% of the time.

## 4. Expected vs Actual

- Expected: A step with a `failed` status shows a red warning indicator (`AlertCircle`). A step with a `needs_input`, `needs_auth`, `reconnect_required`, or `config_stale` status shows an orange warning indicator (`TriangleAlert`). Step 7 tracks the actual provider setup configurations.
- Actual: Stepper indicators only had checkmark (green) or active (green outline/square) or neutral (gray) states. Step 7 tracked the proxy MCP status directly.

## 5. Impact

- Users affected: Admins configuring Google Drive integration.
- Workflows affected: Setup and configuration of the Google Drive proxy MCP and provider integrations.
- Severity: Minor (UX improvement).

## 6. Root Cause

- Hypothesis: Stepper component rendering only checks `isStepConfigured(stepStatus)` and `activeStep === step.index` to determine CSS classes and icons.
- Confirmed cause: The render block in `google-drive-setup.tsx` had no logic for checking `failed` or warning statuses.
- Evidence: Code inspection of `google-drive-setup.tsx` lines 500-550.

## 7. Fix Strategy

- `F-1` Import `AlertCircle` and `TriangleAlert` icons from `lucide-react`.
- `F-2` Update `providerSetupStepStatus` to recognize and return `config_stale` if any provider configuration is stale.
- `F-3` Modify Step 7's status assignment to use `providerSetupStatus` (provider configuration status) instead of `status?.mcp.status`.
- `F-4` Update stepper rendering to check for `isError` (`failed`) and `isWarning` (`needs_input`, `needs_auth`, `reconnect_required`, `config_stale`). Render border-danger/text-danger/ring-danger/20 and border-warning/text-warning/ring-warning/20 accordingly, and display the corresponding icons.

## 8. Validation

- `V-1` Add a unit test `renders error/warning indicator states on steps if they have error/warning statuses` in `google-drive-setup.test.tsx` verifying that `.lucide-circle-alert` and `.lucide-triangle-alert` SVGs are correctly rendered when there are failed and stale configurations.
- `V-2` Run `npx vitest run google-drive-setup.test.tsx` to verify all tests pass.

## 9. Regression Guard

- Tests: `google-drive-setup.test.tsx` verified via Vitest.
- Audit checks: Pre-commit checks to ensure the step indicators compile and render without error.

## 10. Follow-Up Document Updates

- Upstream docs that must change: None.
- Notes left unchanged on purpose: None.
