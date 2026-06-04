# CA-040: Google Drive Runtime Config Review Loop

## Scope

- Local-runner Google Drive workspace config persistence, MCP credential handling, and auth-state validation in `apps/local-runner/internal/runner`.
- Local-runner Google Drive config HTTP endpoints in `apps/local-runner/internal/cli/root.go`.
- Admin-web Google Drive runtime fallback/status helpers in `apps/admin-web/src/app/api/runtime/google-drive-config`.
- Admin-web Google Drive setup UI in `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`.

## Completed

- Added reconnect-aware Google OAuth error classification so artifact sync invalid-grant responses now surface a reconnect-specific state instead of a generic failure message.
- Updated Google Drive artifact connection/session persistence to write `reconnect_required` when stored auth is expired or revoked.
- Tightened MCP OAuth JSON validation so the CP-28 upload flow accepts Desktop OAuth client JSON only and rejects Web OAuth client JSON.
- Added MCP token-file inspection and refresh-token validation so Google Drive MCP readiness no longer relies on token-file existence alone.
- Updated MCP status handling so revoked/expired tokens surface `reconnect_required`, missing token state stays `needs_auth`, and status refresh messaging reflects the actual auth state.
- Cleared the Google Drive setup page secret inputs after successful artifact-sync and Picker-key saves.
- Aligned the admin-web fallback validator with the runner by treating Web OAuth JSON as invalid for the MCP upload flow.
- Expanded targeted runner coverage for env precedence, MCP JSON validation, artifact reconnect handling, and MCP reconnect detection.

## Verification

- Passed `go test ./internal/runner -run "Test(GoogleDrive|UploadGoogleDriveMcpOAuthCredentials|LoadGoogleDriveWorkspaceConfig|ResolveGoogleDriveArtifactRuntimeConfig|HandleGoogleDriveArtifactOAuthCallback|SyncArtifactGoogleDrive|ResolveArtifactOpenURLReturnsGoogleDriveViewURL)"` from `apps/local-runner`.
- Passed `git diff --check` on the CP-28 fix files.
- Re-indexed GitNexus with `npx gitnexus analyze` so impact checks covered the new Google Drive config symbols before the final fix pass.
- Ran GitNexus impact analysis on the Google Drive config/runtime symbols touched in the loop; all reported LOW risk with no indexed caller/process blast radius.
- Completed a reviewer sub-agent pass with final result: clean pass.

## Residual Notes

- Frontend behavior for the updated Google Drive setup messaging and fallback status handling was validated by code review only; no dedicated admin-web test or build was run in this loop.
- Full `go test ./...` for `apps/local-runner` was not rerun; verification stayed scoped to the Google Drive runner paths touched here.
