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
- Extended Google Drive MCP provider-account discovery for Codex, Gemini, and Claude so the runner now surfaces legacy/default homes, managed account slots, and per-account provider-config status rows to the setup flow.
- Updated provider-config repair so invalid Codex/Gemini/Claude config files are rewritten into the expected Google Drive MCP server shape instead of dead-ending on parse errors.
- Aligned provider-config status detection with the written server shape so drift in read-only tool allowlists, timeout fields, approval mode, enable flags, and command/type fields now returns `config_stale` instead of a false `configured`.
- Tightened Gemini discovery validation so metadata-only files, arbitrary JSON payloads, and nested wrapper false positives no longer fabricate discoverable account homes.
- Updated the admin-web provider configuration card so failed provider rows can be retried, partial success is messaged correctly when some providers still lack discovered homes, and the card only acts on actionable provider rows.
- Added targeted backend and frontend regressions for managed account discovery, provider-config status drift, invalid config rewrite, Gemini JSON validation edge cases, and provider configuration UI success/error flows.
- Added Phase B workflow runtime preflight and prompt augmentation for `requiredMcps: ["google_drive"]` so one-shot prompt execution and live session message paths now inject explicit Google Drive MCP instructions and fail before provider launch when auth/config prerequisites are missing.
- Extended local-runner prompt/session result handling to propagate `actualPromptText`, preserve account-home context for provider execution, and downgrade runs to `failed` when providers end their response with an explicit `MCP_FAILURE_CODE: ...` marker.
- Hardened workflow session retry behavior so deterministic MCP setup/preflight failures do not trigger `session_dead` recovery or bootstrap replay, while true transport/thread recovery paths remain intact.
- Narrowed MCP failure parsing to an unquoted terminal marker contract, preventing false failures when providers quote guidance while still accepting legitimate `explanation + marker` tails.
- Added targeted regressions for direct `ExecutePrompt`, session `SendMessage`, prompt preflight branches (`needs_auth`, `reconnect_required`, `config_stale`), explicit-marker parsing, and workflow runtime retry classification.

## Verification

- Passed `go test ./internal/runner -run "Test(GoogleDrive|UploadGoogleDriveMcpOAuthCredentials|LoadGoogleDriveWorkspaceConfig|ResolveGoogleDriveArtifactRuntimeConfig|HandleGoogleDriveArtifactOAuthCallback|SyncArtifactGoogleDrive|ResolveArtifactOpenURLReturnsGoogleDriveViewURL)"` from `apps/local-runner`.
- Passed `git diff --check` on the CP-28 fix files.
- Re-indexed GitNexus with `npx gitnexus analyze` so impact checks covered the new Google Drive config symbols before the final fix pass.
- Ran GitNexus impact analysis on the Google Drive config/runtime symbols touched in the loop; all reported LOW risk with no indexed caller/process blast radius.
- Completed a reviewer sub-agent pass with final result: clean pass.
- Passed `go test ./internal/runner -run "TestDiscover|TestEnsure|TestPreflight|TestResolveGoogleDriveMcpProviderStatuses|TestIsValidGemini"` from `apps/local-runner`.
- Passed `npm run test -- src/data/repository/local-first/local-first-workflow-gateway.test.ts src/routes/_authenticated/settings/components/GoogleDriveProviderConfigCard.test.tsx src/routes/_authenticated/settings/mcp-servers/mcp-connect-test.test.tsx` from `apps/admin-web`.
- Passed targeted Gemini validation regressions with `go test ./internal/runner -run "TestIsValidGeminiAccountPath_ArbitraryJSONInvalid|TestDiscoverGeminiAccountHomes_IgnoresArbitraryJSONCandidates|TestDiscoverGeminiAccountHomes_IgnoresStructurallyEmptyJSONCandidates|TestIsValidGeminiAccountPath_WithGeminiDir|TestIsValidGeminiAccountPath_WithSettingsJson"`.
- Completed a bounded coder/reviewer sub-agent loop for the provider-config and Gemini-discovery follow-up fixes, with final reviewer result: `PASS`.
- Passed `go test ./internal/runner -run 'Test(ExecutePrompt|PreparePromptForRequiredMcps|PreflightGoogleDriveMcp|ApplyRequiredMcpFailureStatus|DetectMcpFailureCode|SendMessage.*GoogleDrive)'` from `apps/local-runner`.
- Passed `go test ./internal/runner -run 'TestProviderDrivenMcpTestPopulatesResult|TestDetectMcpFailureCodePatterns'` from `apps/local-runner`.
- Passed `npm test -- --run src/features/workflow-engine/workflow-start-runtime.test.ts` from `apps/admin-web`.
- Completed a reviewer sub-agent pass on the final Phase B MCP runtime scope with result: clean pass and no actionable correctness findings.

## Residual Notes

- Frontend behavior for the updated Google Drive setup messaging and fallback status handling was validated by code review only; no dedicated admin-web test or build was run in this loop.
- Full `go test ./...` for `apps/local-runner` was not rerun; verification stayed scoped to the Google Drive runner paths touched here.
- Full `npx tsc --noEmit` in `apps/admin-web` still reports unrelated pre-existing errors outside the Google Drive provider-config files, including `src/data/repository/local-first/local-first-workflow-gateway.ts`.
- Gemini discovery now intentionally uses a conservative shape check for `settings.json` and `oauth*.json`; if the upstream CLI introduces additional legitimate top-level schemas, the allowlist may need another small update.
- Workflow-level failure propagation for MCP-marked step results is covered through the lower-level runtime/request tests and the existing `result.status !== "success"` guards, but this loop did not add dedicated end-to-end step-level regressions for `submitWorkflowStepFollowUpRuntime` or `runWorkflowStartRuntime`.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: CP-28
change_type: feature
summary: Google Drive Runtime Config Review Loop
# --->8---
