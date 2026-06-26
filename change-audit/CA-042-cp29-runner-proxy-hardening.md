# CA-042 CP-29 Runner Proxy Hardening

## Scope

Closed the runner-side CP-29 gaps around proxy preflight, approval expiry, and approval scoping so the Google Drive proxy path now validates the real local prerequisites, binds write approvals to the live session deadline, and refuses to persist approvals without workflow/run/process identifiers.

## Completed

- Tightened `googleDriveProxyMcpAuthReady()` and `PreflightGoogleDriveMcp()` so proxy readiness now checks the actual artifact-sync OAuth config, the single connected Drive project, and the stored project refresh token before enabling provider configuration or prompt execution.
- Bound proxy write-approval expiry to the live session deadline instead of the previous fixed two-hour TTL.
- Added write-approval scope validation so proxy write requests reject missing workflow run, workflow step, or process identifiers before persisting approval records.
- Kept approval retry behavior exact-match only, so approved writes and rejected writes continue to use the canonicalized argument hash/summary path.
- Expanded runner-side tests for proxy preflight, session-bound approval expiry, missing scoping identifiers, and prompt-injection approval guidance.

## Verification

- `go test ./internal/runner -run 'TestPreflightGoogleDriveMcp|TestPreparePromptForRequiredMcps|TestProxyMcp|TestResolveWriteApproval|TestHandleWriteTool|TestInjectRequiredMcpInstructions'`
- `go test ./internal/runner -run 'TestPreflightGoogleDriveMcp_ProxyPathDoesNotRequireLegacyDesktopMcpAuth|TestResolveWriteApproval_UsesLiveSessionDeadlineWhenShorterThanTwoHours|TestResolveWriteApproval_UsesLiveSessionDeadlineWhenLongerThanTwoHours|TestResolveWriteApproval_RejectsMissingScopingIdentifiers'`
- `npm test -- workflow-start-runtime`
- `npm test -- workflow-engine-mappers supabase-workflow-engine-gateway workflow-start-runtime && npm run build`
- `go test ./internal/runner -run 'TestEnsure|TestResolveGoogleDriveMcpProviderStatuses|TestInjectRequiredMcpInstructions|TestPreparePromptForRequiredMcps|TestPreflightGoogleDriveMcp|TestProxyMcp|TestResolveWriteApproval|TestHandleWriteTool|TestDetectMcpFailureCodePatterns|TestGenerateMcpVerificationPromptContent'`

## Residual Notes

- Follow-up fix on `2026-06-09` closed the remaining CP-29 workflow `mcpAccessMode` model and audit-artifact path.
- Final review pass on `2026-06-09` tightened the rejected Google Drive approval test to assert the pending approval lookup and workflow-step join explicitly.
- The proxy approval deadline now matches the current session deadline, so approvals on nearly expired sessions will expire quickly by design.
- Runner-side tests now inject the in-memory secret store in CP-29 fixtures and do not touch the macOS keychain.

# ---8<--- flowpilot:change-ledger
feature_key: google-drive
source_doc_id: CP-29
change_type: feature
summary: CP-29 Runner Proxy Hardening
# --->8---
