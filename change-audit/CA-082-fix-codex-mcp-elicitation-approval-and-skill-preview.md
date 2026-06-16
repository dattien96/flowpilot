# CA-082: Fix Codex MCP Elicitation Approval And Skill Preview

## Scope

- Codex inbound MCP elicitation handling in `apps/local-runner/internal/runner/codex_adapter.go`.
- Codex app-server tests in `apps/local-runner/internal/runner/codex_appserver_test.go`.
- Desktop prompt skill summary preview in `apps/desktop-flowpilot/src/components/Timeline.tsx`.
- BUG records `BUG-071` and `BUG-072`.

## Completed

- Restored the desktop collapsed selected-skill preview to the first two skills plus a remainder count.
- Preserved full selected skill delivery through `injectSelectedSkills` and its explicit selected-name list.
- Added `mcpServer/elicitation/request` handling through the existing approval bridge.
- Added method-aware approval details for MCP elicitations using `serverName` and `message`.
- Added Codex MCP elicitation response mapping to `{ action, content, _meta }`.

## Verification

- `npx gitnexus impact codexInboundApprovalMethod --direction upstream` reported LOW risk.
- `npx gitnexus impact codexApprovalResponse --direction upstream` reported LOW risk.
- `npx gitnexus impact PromptSkillsSummary --direction upstream` reported LOW risk.
- `go test ./internal/runner -run "Test(CodexAdapterMcpElicitationApprovalRoundTrip|CodexAdapterPermissionsApprovalRoundTrip|CodexAdapterYoloApprovalMatrix|ResolveYoloPosture|InjectSelectedSkillsDeliversSelection)$" -count=1` passed.
- `npm run typecheck --prefix apps/desktop-flowpilot` passed.

## Residual Notes

- `item/tool/call` is still unsupported and should be handled separately if Codex requires FlowPilot to execute dynamic client tools.
