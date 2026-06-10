# CA-043 CP-29 Google Drive MCP Manual Approval UI

## Scope

Closed the remaining CP-29 non-YOLO approval gaps in the admin-web run-detail surfaces and aligned the CP-29 plan with the shipped headless proxy design.

## Completed

- Updated the Google Drive MCP approval HTTP handler so it now supports both bearer-auth JSON requests from the browser gateway and cookie-auth form posts from the server-rendered workflow run page.
- Fixed the interactive workflow run page so Google Drive MCP waiting steps use the MCP approve/reject flow instead of the old generic `Reject & Retry` review path.
- Fixed the server-rendered workflow run page approval panel so Google Drive MCP waiting steps post to the dedicated MCP approval endpoint instead of the generic `/api/approvals/...` flow.
- Revised `CP-29` to match the implemented architecture: provider-host approval stays non-blocking in headless runs, FlowPilot proxy owns exact read/write approvals, and YOLO intent is carried by proxy runtime config rather than provider-host popups.
- Reviewed `CP-30` during the loop and left it unchanged because its account-binding model still matches the implemented approval path.

## Verification

- `npm test -- workflow-start-runtime.test.ts submit-google-drive-write-approval-http-handler.test.ts`
- `npm run build`

## Residual Notes

- GitNexus CLI in this environment exposes `impact`, `query`, and `context`, but not the `detect_changes` command named in `AGENTS.md`; scope review was done with `git diff` after impact analysis.
- The proxy approval route still uses the legacy `google-drive-write-approval` path name for compatibility, even though it now handles both read and write MCP approvals.
