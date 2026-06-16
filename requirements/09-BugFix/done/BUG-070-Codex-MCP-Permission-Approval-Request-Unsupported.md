# BUG-070: Codex MCP Permission Approval Request Unsupported

## Metadata

- Document ID: `BUG-070`
- Title: `Codex MCP Permission Approval Request Unsupported`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [CP-29: FlowPilot Proxy MCP Server For Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-066: Codex V2 Approval Decision Rejected By App-Server](./BUG-066-Codex-V2-Approval-Decision-Rejected-By-AppServer.md), [BUG-064: Codex Approval Decision Value Rejected By App-Server](./BUG-064-Codex-Approval-Decision-Value-Rejected-By-AppServer.md), [BUG-061: Codex MCP Server Config Rejects on-request Approval Variant](./BUG-061-Codex-MCP-Config-On-Request-Invalid-Approval-Variant.md), [BUG-032: Codex Google Drive MCP Model Override And Tool Approval](./BUG-032-Codex-Google-Drive-MCP-Model-And-Approval.md), [CA-080: Fix Codex MCP Permission Approval Response](../../../change-audit/CA-080-fix-codex-mcp-permission-approval-response.md)
- Replaces: `none`
- Tags: `codex, mcp, approval, app-server, google-drive, regression, high`

## AI Quick View

### Summary

- BUG-066 fixed command/file app-server approval methods but left `item/permissions/requestApproval` out of scope.
- With YOLO off, Codex can send that permission approval request before using configured MCP tools such as Google Drive.
- The adapter treated the method as unsupported, so no FlowPilot approval card was shown and Codex reported tool-call failure.
- YOLO on avoided the path because Codex did not need to ask for the extra permission.

### Current Ask

- Done. Codex permission approval requests now route through the existing approval bridge and reply with the generated Codex permission response shape.

### Key Decisions

- `V-1` Keep FlowPilot UI decisions as `approve` / `deny`; translate only at the Codex adapter boundary.
- `V-2` `item/permissions/requestApproval` must not reuse command/file `{ decision: ... }` responses.
- `V-3` Approved permission requests return `{ permissions, scope }`, granting the requested network/fileSystem profile for the current turn unless the user chose session approval.
- `V-4` Denied, expired, or unknown decisions return an empty permission profile with `scope: "turn"` so Codex does not hang.

### Constraints

- Do not widen the desktop approval DTO for Codex-specific permission-profile details in this bug.
- Preserve existing legacy and v2 command/file approval mappings from BUG-064 and BUG-066.
- GitNexus MCP tools were not exposed, but the local GitNexus CLI was used after refreshing the stale index.

### Open Questions

- Whether the desktop approval card should later show a richer summary of Codex permission-profile fields for MCP access.

### Source Refs

- User report on `2026-06-16`: after BUG-066, command/file approval works, YOLO-on MCP works, but YOLO-off MCP fails without showing approval.
- Generated local Codex schema from `codex app-server generate-ts --out <temp>` on `2026-06-16`.
- Runner code: `apps/local-runner/internal/runner/codex_adapter.go`.

## 1. Issue Summary

Codex desktop chat approval worked for shell/file operations after BUG-066, but MCP Google Drive usage still failed when YOLO was off. The user expected a normal approval card, as with writing files, but no approval surfaced and the tool call failed.

## 2. Parent Links

- impacted coding plan: [CP-29: FlowPilot Proxy MCP Server For Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` chat mode with Codex app-server, Google Drive MCP configured, YOLO off.
- reproduction steps:
  1. Configure Google Drive MCP and Codex app-server chat.
  2. Leave YOLO off.
  3. Ask Codex to interact with Google Drive through MCP.
  4. Observe the MCP tool call fail without a FlowPilot approval card.
  5. Turn YOLO on and retry; MCP succeeds.
- frequency: reproducible when Codex emits `item/permissions/requestApproval` for MCP access.

## 4. Expected vs Actual

- expected: YOLO-off MCP permission requests should show the same FlowPilot approval gate pattern used for command/file approvals, and approval should unblock Codex.
- actual: the adapter returned an unsupported inbound-request error for `item/permissions/requestApproval`, so no approval card was shown.

## 5. Impact

- users affected: desktop chat users running Codex with MCP tools configured and YOLO off.
- workflows affected: MCP tool calls that require Codex app-server permission approval, including Google Drive access.
- severity: high, because the safe non-YOLO path blocks MCP usage while YOLO works.

## 6. Root Cause

- hypothesis: the previous app-server approval fixes handled command/file request methods but not Codex's separate permission request method.
- confirmed cause: generated Codex app-server schema includes `item/permissions/requestApproval` with `PermissionsRequestApprovalResponse`, whose response is `{ permissions: GrantedPermissionProfile, scope: PermissionGrantScope, strictAutoReview?: boolean }`. The adapter only recognized command/file methods and rejected this request as unsupported.
- evidence:
  - `ServerRequest.ts` lists `item/permissions/requestApproval`.
  - `PermissionsRequestApprovalResponse.ts` requires `permissions` and `scope`, not `decision`.
  - New test `TestCodexAdapterPermissionsApprovalRoundTrip` fails without the handler and passes with it.

## 7. Fix Strategy

- `F-1` Add `item/permissions/requestApproval` to the Codex inbound approval method allowlist.
- `F-2` Replace the single `{ decision: ... }` reply construction in `handleInbound` with a method-specific approval response builder.
- `F-3` For command/file methods, preserve the BUG-064 and BUG-066 decision enum mappings.
- `F-4` For permission approvals, grant the requested `network` and `fileSystem` permission profile on approve and return an empty permission profile on deny or expiry.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(CodexAdapterApprovalRoundTrip|CodexAdapterV2ApprovalRoundTrip|CodexAdapterPermissionsApprovalRoundTrip|CodexAdapterYoloApprovalMatrix|CodexReviewDecisionMapsToAppServerEnum|CodexReviewDecisionMapsV2AppServerEnum)$" -count=1` passes.
- `V-2` `go build ./...` passes in `apps/local-runner`.
- `V-3` `go test ./internal/runner -count=1` was run for broader signal and still fails on unrelated pre-existing Google Drive config/provider-home/registry tests.
- `V-4` Local GitNexus index was refreshed with `npx gitnexus analyze`; impact checks on `codexInboundApprovalMethod`, `codexApprovalResponse`, and `codexPermissionsApprovalResponse` reported LOW risk.

 The Codex logic is now covered as:

  - Write/command tool + YOLO off: workspace-write + untrusted, approval bridge is called.
  - Write/command tool + YOLO on: danger-full-access + never, no approval bridge call.
  - MCP permission + YOLO off: workspace-write + untrusted, approval bridge is called and replies with { permissions,
    scope }.

  - MCP permission + YOLO on: danger-full-access + never, no approval bridge call.


## 9. Regression Guard

- tests: `TestCodexAdapterPermissionsApprovalRoundTrip` covers the v2 permission response shape; `TestCodexAdapterYoloApprovalMatrix` covers command/file write and MCP permission behavior with YOLO on and off; existing command/file approval tests cover legacy and v2 decision mappings.
- alerts: none.
- audit checks: [CA-080](../../../change-audit/CA-080-fix-codex-mcp-permission-approval-response.md) records the implementation and verification.

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`; this corrects Codex app-server wire compatibility and does not change product approval behavior.
- notes left unchanged on purpose:
  - Desktop approval cards remain provider-neutral and still speak FlowPilot's `approve` / `deny` vocabulary.
  - Rich display of requested permission-profile details remains a possible follow-up UI enhancement.
