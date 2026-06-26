# CA-080: Fix Codex MCP Permission Approval Response

## Scope

- Fixed Codex app-server inbound approval handling in `apps/local-runner/internal/runner/codex_adapter.go`.
- Added regression coverage in `apps/local-runner/internal/runner/codex_appserver_test.go`.
- Documented the resolved bug as `BUG-070`.

## Completed

- Added support for Codex v2 `item/permissions/requestApproval`.
- Split Codex approval replies by request method so command/file approvals still return `{ decision: ... }`, while permission approvals return `{ permissions, scope }`.
- Mapped approved permission requests to the requested `network` and `fileSystem` permission profile.
- Mapped denied, expired, or unknown permission decisions to an empty permission profile so Codex does not hang or gain extra access.
- Added an explicit Codex four-case matrix: write-tool YOLO off, write-tool YOLO on, MCP permission YOLO off, and MCP permission YOLO on.
- Verified the installed Codex schema with `codex app-server generate-ts --out C:\Users\dat.nguyen\AppData\Local\Temp\codex-appserver-schema`.

## Verification

- `npx gitnexus analyze` refreshed the stale local index.
- `npx gitnexus impact codexInboundApprovalMethod --direction upstream` reported LOW risk, 0 direct upstream callers, and 0 affected processes.
- `npx gitnexus impact codexApprovalResponse --direction upstream` reported LOW risk, 0 direct upstream callers, and 0 affected processes.
- `npx gitnexus impact codexPermissionsApprovalResponse --direction upstream` reported LOW risk, 1 direct caller, and 0 affected processes.
- `go test ./internal/runner -run "Test(CodexAdapterApprovalRoundTrip|CodexAdapterV2ApprovalRoundTrip|CodexAdapterPermissionsApprovalRoundTrip|CodexAdapterYoloApprovalMatrix|CodexReviewDecisionMapsToAppServerEnum|CodexReviewDecisionMapsV2AppServerEnum)$" -count=1` passed.
- `go build ./...` from `apps/local-runner` passed.
- `go test ./internal/runner -count=1` was run and failed on unrelated existing Google Drive config/provider-home/registry tests.

## Residual Notes

- The desktop approval DTO was not expanded to show the raw permission-profile object; the current approval card still shows provider-neutral command/cwd/reason fields.
- The GitNexus CLI does not expose the MCP-only `gitnexus_detect_changes()` command in this session, so pre-commit detect-changes was not run.

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: BUG-070
change_type: fix
summary: Fix Codex MCP Permission Approval Response
# --->8---
