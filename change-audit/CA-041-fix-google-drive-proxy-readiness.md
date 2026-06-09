# CA-041 Fix Google Drive Proxy Readiness

## Scope

Tightened the CP-29 Google Drive proxy MCP path so provider setup, MCP preflight, and the admin-web setup card all follow the active proxy feature flag instead of guessing from artifact-sync readiness alone.

## Completed

- Updated local-runner provider-config readiness in `google_drive_mcp_provider_config.go` to treat proxy mode as artifact-sync OAuth based instead of raw `google-drive-mcp` desktop auth based.
- Updated MCP preflight in `mcp_prompt_instructions.go` so workflow/session prompt preparation follows the same proxy readiness rule instead of failing on missing legacy desktop MCP credentials.
- Added `proxyMcpEnabled` to the Google Drive runtime status payload so admin-web can distinguish proxy-mode readiness from legacy raw-MCP readiness.
- Updated the admin-web provider setup card to require both `proxyMcpEnabled` and artifact-sync OAuth readiness before enabling configuration.
- Aligned the browser fallback helper in `apps/admin-web/src/app/api/runtime/google-drive-config/_shared.ts` so a valid credential path plus refresh-token file reports `configured`, and made the path expectation test platform-safe.
- Corrected the CP-29 `## 10. Definition of Done` checklist so the current proxy slice is marked accurately and does not overclaim manual write approval, rejection handling, audit artifacts, or full test coverage.
- Added `## 11. Test Guide` to CP-29 with step-by-step manual verification for all planned requirements, split between checks that should pass now and future acceptance checks that still represent open work.
- Added explicit runner coverage for:
  - proxy provider-config setup without legacy desktop MCP auth
  - MCP preflight without legacy desktop MCP auth
  - proxy server access-token resolution through artifact-sync stored project credentials
  - multiple artifact-sync connection rejection
  - proxy auth status reporting through the artifact-sync source
  - proxy feature-flag propagation into the runtime status payload
  - proxy-disabled UI gating in the admin-web setup card

## Verification

- `go test ./internal/runner -run 'TestProxyMcp|TestPreflightGoogleDriveMcp|TestEnsureGoogleDriveMcpProviderConfig|TestPreparePromptForRequiredMcps'`
- `go test ./internal/runner -run 'TestLoadGoogleDriveWorkspaceConfigUsesEnvFallback|TestGoogleDriveWorkspaceStatusHidesSecretsFromJSONResponse|TestPreflightGoogleDriveMcp|TestEnsureGoogleDriveMcpProviderConfig|TestProxyMcp'`
- `npm test -- --run src/routes/_authenticated/settings/components/GoogleDriveProviderConfigCard.test.tsx src/app/api/runtime/google-drive-config/_shared.test.ts src/presentation/components/artifacts/artifact-cloud-storage-panel.test.tsx`
- `go test ./internal/runner -run 'TestRunProviderDrivenMcpTest|TestProviderDrivenMcpTest'`

## Residual Notes

- The proxy server now uses artifact-sync stored Google credentials, but until explicit run/project scoping is passed into the proxy process it only auto-selects a single connected project. If more than one Google Drive artifact-sync connection is present locally, the proxy returns a clear error instead of guessing.
- CP-29 is still only partially delivered beyond the readiness and provider-config slice. Manual write approval enforcement, rejection handling, approval UI, write audit artifacts, and full end-to-end validation coverage remain open work.
- The GitNexus MCP impact/detect tools were not exposed in this tool environment, so scope verification used repo-local search plus focused test coverage after re-running `npx gitnexus analyze`.
