# BUG-061: Codex MCP Server Config Rejects `on-request` Approval Variant

## Metadata

- Document ID: `BUG-061`
- Title: `Codex MCP Server Config Rejects on-request Approval Variant`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [CP-30: Google Drive Account Connection And Artifact Folder Binding](../../07-Coding-Plan/done/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md), [CP-29: MCP Proxy Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md)
- Child Documents: `none`
- Related Documents: [BUG-032: Codex Google Drive MCP Model And Approval](./BUG-032-Codex-Google-Drive-MCP-Model-And-Approval.md), [Task-043: Desktop Google Drive Setup Tab](../../08-Task/done/Task-043-Desktop-Google-Drive-Setup-Tab.md)
- Replaces: `none`
- Tags: `google-drive-mcp, codex, approval-mode, runner, regression`

## AI Quick View

### Summary

- Clicking "Configure AI Providers" in the desktop Google Drive setup wizard (Step 7) produces `Error creating task: invalid configuration: unknown variant 'on-request', expected one of 'auto', 'prompt', 'approve' in 'mcp_servers.google-drive.default_tools_approval_mode'`.
- The Codex **MCP server config** TOML field `default_tools_approval_mode` and the Codex **thread/task-level** `approvalMode`/`approvalPolicy` API are two distinct enums; the runner was writing `"on-request"` into the TOML field, which is only valid at the task level.
- `googleDriveProxyMcpApprovalMode(false)` in `google_drive_mcp_provider_config.go` returned `"on-request"` (the task-level value) where the MCP server config TOML requires `"prompt"`.
- The thread-level `CodexApprovalMode: "on-request"` in `yolo_resolver.go` is correct and was not changed.

### Current Ask

- Change `googleDriveProxyMcpApprovalMode(false)` to return `"prompt"` and update the three affected test assertions.

### Key Decisions

- `V-1` The two Codex approval enums are separate: task-level uses `{untrusted, on-failure, on-request, granular, never}`; MCP server TOML uses `{auto, prompt, approve}`. Each must use its own correct variant.
- `V-2` `"prompt"` in the MCP server config is the semantic equivalent of `"on-request"` at the task level — Codex surfaces a `permission_required` event which FlowPilot's approval bridge handles.
- `V-3` The read-only direct-path approval mode (`"approve"` at line 593, fixed in BUG-032) is a separate code path and was not changed.

### Constraints

- Do not change `yolo_resolver.go` `CodexApprovalMode: "on-request"` — it is passed to the Codex `thread/start` RPC where `"on-request"` is a valid variant.
- Keep YOLO=true → `"approve"` unchanged for the proxy MCP path.

### Open Questions

- `none`

### Source Refs

- Error seen at desktop Google Drive setup Step 7 → "Configure AI Providers" button.
- Runner code: `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go` `googleDriveProxyMcpApprovalMode`
- YOLO SSOT: `apps/local-runner/internal/runner/yolo_resolver.go`

## 1. Issue Summary

When a user clicks "Configure AI Providers" in the desktop Google Drive setup wizard (Step 7), the runner writes a Codex config TOML entry for the `google-drive` MCP server containing `default_tools_approval_mode = "on-request"`. Codex rejects this with:

```
Error creating task
invalid configuration: unknown variant `on-request`, expected one of `auto`, `prompt`, `approve`
in `mcp_servers.google-drive.default_tools_approval_mode`
```

The task creation fails entirely, preventing the proxy MCP provider config from being applied.

## 2. Parent Links

- impacted coding plan: [CP-30: Google Drive Account Connection And Artifact Folder Binding](../../07-Coding-Plan/done/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md)
- impacted tech design: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- impacted system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)

## 3. Environment and Reproduction

- environment: desktop FlowPilot app, Google Drive setup wizard Step 7, runner running locally
- reproduction steps:
  1. Complete Google Drive setup steps 1–6 in the desktop settings.
  2. Open Step 7 (Proxy MCP setup).
  3. Click "Configure AI Providers".
  4. Observe error modal: `invalid configuration: unknown variant 'on-request'`.
- frequency: 100% reproducible with YOLO=false (normal non-yolo runs).

## 4. Expected vs Actual

- expected: Runner writes `default_tools_approval_mode = "prompt"` to the Codex MCP server config TOML; Codex accepts the config and the proxy MCP provider entries are applied.
- actual: Runner writes `default_tools_approval_mode = "on-request"` which Codex rejects with an unknown variant error; provider config is not applied.

## 5. Impact

- users affected: all users configuring Google Drive proxy MCP from the desktop app with YOLO=false.
- workflows affected: Google Drive proxy MCP provider config setup; any Codex-backed workflow step that depends on a correctly configured Google Drive MCP server.
- severity: high — blocks the entire "Configure AI Providers" flow.

## 6. Root Cause

- hypothesis: Codex changed the accepted enum set for `default_tools_approval_mode` independently of the task-level `approvalMode` API.
- confirmed cause: `googleDriveProxyMcpApprovalMode(false)` returned `"on-request"`, which is only valid in the Codex thread/start task-level API (`{untrusted, on-failure, on-request, granular, never}`). The MCP server config TOML field `default_tools_approval_mode` uses a different enum (`{auto, prompt, approve}`).
- evidence:
  - Error message explicitly names both the rejected value and the valid set.
  - `codexThreadStartParams` in `codex_appserver.go` sets `p["approvalMode"] = approvalMode` from `CodexApprovalMode` (task level) — a separate field from the TOML `default_tools_approval_mode`.
  - Reverting `yolo_resolver.go` `CodexApprovalMode` back to `"on-request"` (task level) produced the second error (`unknown variant 'prompt', expected one of 'untrusted', 'on-failure', 'on-request', ...`), confirming the two APIs are separate enums.

## 7. Fix Strategy

- `F-1` In `google_drive_mcp_provider_config.go`, change `googleDriveProxyMcpApprovalMode(false)` to return `"prompt"` instead of `"on-request"`. Update the comment to reference `"prompt"`.
- `F-2` Leave `yolo_resolver.go` `CodexApprovalMode: "on-request"` unchanged — it is correct for the task-level API.
- `F-3` Update three test assertions in `google_drive_mcp_provider_config_test.go` and one in `phase4_test.go` that checked for `"on-request"` in the MCP server config context.

## 8. Validation

- `V-1` `go test ./internal/runner/... -run "TestResolveYoloPosture|TestGoogleDrive"` passes (10 tests).
- `V-2` Manual smoke: desktop Step 7 "Configure AI Providers" no longer shows the unknown variant error.

## 9. Regression Guard

- tests:
  - `TestResolveYoloPosture` — asserts `CodexApprovalMode == "on-request"` for task level (unchanged).
  - `TestGoogleDriveMcpProviderConfigEnsure` — asserts `ApprovalMode == "prompt"` for proxy path yolo=false.
  - `TestEnsureCodexGoogleDriveMcpConfig` — asserts proxy rewrite approval mode is `"prompt"` for yolo=false.
- alerts: none — this is config generation, not a runtime data path.
- audit checks: confirm generated Codex config TOML contains `default_tools_approval_mode = "prompt"` for proxy path with YOLO=false; confirm `"approve"` for YOLO=true.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — the two-enum split is an implementation detail of the Codex API, not a change to FlowPilot's design intent.
- notes left unchanged on purpose: `SD-11` and `SS-05` do not prescribe specific Codex enum values; no update needed.
