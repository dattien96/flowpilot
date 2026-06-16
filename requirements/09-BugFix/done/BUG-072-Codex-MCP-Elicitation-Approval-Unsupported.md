# BUG-072: Codex MCP Elicitation Approval Unsupported

## Metadata

- Document ID: `BUG-072`
- Title: `Codex MCP Elicitation Approval Unsupported`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [CP-29: FlowPilot Proxy MCP Server For Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)
- Child Documents: `none`
- Related Documents: [BUG-070: Codex MCP Permission Approval Request Unsupported](./BUG-070-Codex-MCP-Permission-Approval-Request-Unsupported.md), [BUG-071: Codex YOLO-Off Workspace Write And Selected Skill Names Regressed](./BUG-071-Codex-Yolo-Off-Workspace-Write-And-Selected-Skill-Names-Regressed.md), [CA-082: Fix Codex MCP Elicitation Approval And Skill Preview](../../../change-audit/CA-082-fix-codex-mcp-elicitation-approval-and-skill-preview.md)
- Replaces: `none`
- Tags: `codex, mcp, approval, elicitation, google-drive, regression, high`

## AI Quick View

### Summary

- Retesting MCP with YOLO off still failed without showing an approval UI.
- BUG-070 handled `item/permissions/requestApproval`, but Codex can also send `mcpServer/elicitation/request`.
- The adapter still rejected that method as unsupported, so MCP confirmation never reached FlowPilot's approval card.

### Current Ask

- Done. MCP server elicitations now route through the approval bridge and reply with Codex's `{ action, content, _meta }` response shape.

### Key Decisions

- `V-1` Treat `mcpServer/elicitation/request` as a user approval request.
- `V-2` Map approve to `action: "accept"`, deny to `action: "decline"`, and abort/cancel to `action: "cancel"`.
- `V-3` Show meaningful approval details using `serverName` and `message`.

### Constraints

- Preserve BUG-070 permission-profile response handling.
- Do not implement arbitrary `item/tool/call` execution in this bug.

### Open Questions

- Whether Codex `item/tool/call` should get a separate implementation for FlowPilot-owned dynamic tools such as `ask_user`.

### Source Refs

- User retest on `2026-06-16`: MCP failed without approval UI after write approval was fixed.
- Generated Codex app-server schema: `ServerRequest.ts`, `McpServerElicitationRequestParams.ts`, `McpServerElicitationRequestResponse.ts`.

## 1. Issue Summary

Codex MCP usage with YOLO off still failed without a FlowPilot approval prompt. The prior fix only covered permission-profile approval, not MCP server elicitations.

## 2. Parent Links

- impacted coding plan: [CP-29: FlowPilot Proxy MCP Server For Google Drive](../../07-Coding-Plan/done/CP-29-MCP-Proxy-Google-Drive.md)
- impacted tech design: [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- impacted system spec: [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md)

## 3. Environment and Reproduction

- environment: desktop chat mode with Codex app-server, Google Drive MCP configured, YOLO off.
- reproduction steps:
  1. Ask Codex to use the configured MCP Drive integration.
  2. Observe MCP failure without approval UI.
- frequency: reproducible when Codex emits `mcpServer/elicitation/request`.

## 4. Expected vs Actual

- expected: MCP server confirmation requests show FlowPilot approval UI and resume after approve/deny.
- actual: the adapter returned unsupported inbound request and no approval card appeared.

## 5. Impact

- users affected: Codex desktop chat users using MCP tools with YOLO off.
- workflows affected: MCP tools that require server elicitation.
- severity: high.

## 6. Root Cause

- hypothesis: another Codex app-server MCP request variant remained unhandled.
- confirmed cause: generated schema lists `mcpServer/elicitation/request`, whose response is `{ action, content, _meta }`. The adapter only allowed command/file approval and permission approval methods.
- evidence: new `TestCodexAdapterMcpElicitationApprovalRoundTrip` covers approval details and response shape.

## 7. Fix Strategy

- `F-1` Add `mcpServer/elicitation/request` to the inbound approval method allowlist.
- `F-2` Build approval details from `serverName` and `message`.
- `F-3` Add `codexMcpElicitationApprovalResponse` with `accept | decline | cancel` action mapping.
- `F-4` Add a round-trip regression test.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(CodexAdapterMcpElicitationApprovalRoundTrip|CodexAdapterPermissionsApprovalRoundTrip|CodexAdapterYoloApprovalMatrix|ResolveYoloPosture|InjectSelectedSkillsDeliversSelection)$" -count=1` passes.
- `V-2` `npm run typecheck --prefix apps/desktop-flowpilot` passes.

## 9. Regression Guard

- tests: `TestCodexAdapterMcpElicitationApprovalRoundTrip`.
- alerts: none.
- audit checks: [CA-082](../../../change-audit/CA-082-fix-codex-mcp-elicitation-approval-and-skill-preview.md).

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`; this is Codex app-server wire handling.
- notes left unchanged on purpose:
  - `item/tool/call` remains out of scope.
