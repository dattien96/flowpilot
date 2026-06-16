# BUG-066: Codex V2 Approval Decision Rejected By App-Server

## Metadata

- Document ID: `BUG-066`
- Title: `Codex V2 Approval Decision Rejected By App-Server`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-064: Codex Approval Decision Value Rejected By App-Server](./BUG-064-Codex-Approval-Decision-Value-Rejected-By-AppServer.md), [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [CA-076: Fix Codex V2 Approval Decision Mapping](../../../change-audit/CA-076-fix-codex-v2-approval-decision-mapping.md)
- Replaces: `none`
- Tags: `codex, approval, app-server, runner, regression, high`

## AI Quick View

### Summary

- After BUG-064, clicking **Approve** could still leave Codex reporting `exec command rejected by user`.
- The old fix mapped FlowPilot `approve` to legacy Codex `ReviewDecision` value `approved`.
- Codex 0.137.0 can also send v2 approval method `item/commandExecution/requestApproval`, whose response enum is `accept | acceptForSession | decline | cancel`.
- Returning legacy `approved` to that v2 method is invalid, so the command is still declined.

### Current Ask

- Done. The Codex adapter now maps approval decisions by inbound request method.

### Key Decisions

- `V-1` Preserve FlowPilot UI/bridge decision values (`approve`, `deny`) and translate only at the Codex adapter boundary.
- `V-2` Use legacy `approved | approved_for_session | denied | abort` only for `execCommandApproval`, `applyPatchApproval`, and the old placeholder `approval/request`.
- `V-3` Use v2 `accept | acceptForSession | decline | cancel` for `item/commandExecution/requestApproval` and `item/fileChange/requestApproval`.
- `V-4` Unknown/empty decisions fail closed to the deny value for the active protocol generation.

### Constraints

- GitNexus MCP tools were not exposed in this thread, so symbol impact analysis could not be run.
- Do not change the desktop approval card vocabulary or runner policy engine.
- `item/permissions/requestApproval` uses a different response shape and remains out of scope for this bug.

### Open Questions

- Whether a future task should add full handling for Codex `item/permissions/requestApproval`.

### Source Refs

- User retest on `2026-06-16`: approval card showed `decision: approve`, but Codex still reported the command was rejected.
- Generated local Codex schema from `codex app-server generate-ts` on codex-cli `0.137.0`.
- Runner code: `apps/local-runner/internal/runner/codex_adapter.go`.

## 1. Issue Summary

The BUG-064 fix handled the legacy app-server approval response enum but missed the newer v2 approval method. In the user's retest, Codex still treated the approved shell command as rejected because FlowPilot could reply with a legacy decision value to a v2 approval request.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: `apps/desktop-flowpilot` chat mode + `apps/local-runner` with `FLOWPILOT_CODEX_APPSERVER=1`, Codex CLI `0.137.0`.
- reproduction steps:
  1. Open desktop chat mode with Codex and YOLO off.
  2. Ask Codex to create `yolo-test.txt`.
  3. Click **Approve** on the command approval card.
  4. Observe the timeline still show a declined/failed command, including `exec command rejected by user`.
- frequency: reproducible when Codex sends the v2 command approval request method.

## 4. Expected vs Actual

- expected: clicking Approve sends the valid approval enum for the exact Codex request method, so the command proceeds.
- actual: the adapter could send legacy `approved` to `item/commandExecution/requestApproval`, whose generated response type expects `accept`.

## 5. Impact

- users affected: desktop chat users running Codex with YOLO off.
- workflows affected: Codex command/file approval paths that arrive through v2 app-server request methods.
- severity: high, because safe non-YOLO Codex execution still appears broken after the first approval fix.

## 6. Root Cause

- hypothesis: BUG-064 verified the legacy `execCommandApproval` / `applyPatchApproval` response enum but did not inspect all generated server request variants.
- confirmed cause: Codex app-server schema includes `item/commandExecution/requestApproval` with response type `CommandExecutionRequestApprovalResponse`, whose `decision` is `CommandExecutionApprovalDecision` (`accept | acceptForSession | decline | cancel`). The adapter's single mapper returned legacy `approved`, which is invalid for that v2 method.
- evidence:
  - local generated schema from `codex app-server generate-ts` lists `ServerRequest` variants for both legacy `execCommandApproval` and v2 `item/commandExecution/requestApproval`.
  - generated `v2/CommandExecutionApprovalDecision.ts` lists `accept | acceptForSession | decline | cancel`.
  - new regression test `TestCodexAdapterV2ApprovalRoundTrip` fails without the method-specific mapper and passes with it.

## 7. Fix Strategy

- `F-1` Add `codexInboundApprovalMethod` so the adapter only treats known approval request methods as command/file approvals.
- `F-2` Change `codexReviewDecision` to accept the request method and dispatch to legacy or v2 enum mapping.
- `F-3` Keep legacy mapping for `execCommandApproval`, `applyPatchApproval`, and `approval/request`.
- `F-4` Add v2 mapping for `item/commandExecution/requestApproval` and `item/fileChange/requestApproval`: `approve` -> `accept`, `approve_for_session` -> `acceptForSession`, `deny` -> `decline`, `abort` -> `cancel`.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(CodexAdapterApprovalRoundTrip|CodexAdapterV2ApprovalRoundTrip|CodexReviewDecisionMapsToAppServerEnum|CodexReviewDecisionMapsV2AppServerEnum)" -count=1` passes.
- `V-2` `go build ./...` passes in `apps/local-runner`.
- `V-3` `go test ./internal/runner -count=1` was run for broader signal but has pre-existing unrelated failures in Google Drive credential/config tests, provider-home skill merge tests, and Windows `sh` session tests.

## 9. Regression Guard

- tests: `TestCodexAdapterV2ApprovalRoundTrip`, `TestCodexReviewDecisionMapsV2AppServerEnum`, plus existing legacy approval tests.
- alerts: none.
- audit checks: method-specific protocol mapping reviewed against generated Codex schema; GitNexus detect/impact tooling unavailable in this thread.

## 10. Follow-Up Document Updates

- upstream docs that must change: `none`; this is Codex app-server wire compatibility, not product behavior.
- notes left unchanged on purpose:
  - FlowPilot UI and runner policy continue using `approve` / `deny`.
  - Full handling for `item/permissions/requestApproval` is left as a separate follow-up question because its response does not use a `decision` field.
