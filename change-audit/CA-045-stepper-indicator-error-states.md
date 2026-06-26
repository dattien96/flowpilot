# CA-045: Stepper Indicator Error and Warning States

## Scope

- Google Drive settings setup stepper (`/settings/google-drive-setup`)
- Steps indicator states in `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
- Provider setup configuration status mapping for Step 7

## Completed

- **Stepper indicator states added**: Added `isError` (red/danger) and `isWarning` (orange/warning) styling states to the 7-step stepper indicator on the Google Drive setup page.
- **Icons mapped**: Rendered Lucide's `AlertCircle` (circle-alert) icon for errors and `TriangleAlert` for warnings instead of simple dots.
- **Stale config support**: Enhanced `providerSetupStepStatus` to recognize and return `config_stale` if any of the provider configurations are stale.
- **Step 7 status corrected**: Mapped Step 7's status directly to `providerSetupStatus` (provider configuration status) instead of proxy MCP status.
- **Added unit test**: Added a test verifying warning and error states are rendered properly in `google-drive-setup.test.tsx`.

## Verification

- Ran `npx vitest run google-drive-setup.test.tsx` which successfully passed all 6 tests including the new warning/error state test.

## Residual Notes

- None.

# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-045
change_type: feature
summary: Stepper Indicator Error and Warning States
# --->8---
