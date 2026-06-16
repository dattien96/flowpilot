# BUG-071: Codex YOLO-Off Workspace Write And Selected Skill Names Regressed

## Metadata

- Document ID: `BUG-071`
- Title: `Codex YOLO-Off Workspace Write And Selected Skill Names Regressed`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-16`
- Last Updated: `2026-06-16`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-070: Codex MCP Permission Approval Request Unsupported](./BUG-070-Codex-MCP-Permission-Approval-Request-Unsupported.md), [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [CA-081: Fix Codex YOLO-Off Write Gate And Selected Skill Names](../../../change-audit/CA-081-fix-codex-yolo-off-write-gate-and-selected-skill-names.md)
- Replaces: `none`
- Tags: `codex, yolo, approval, skills, desktop-chat, regression, high`

## AI Quick View

### Summary

- User retest created `yolo-test.txt` without an approval UI while YOLO was expected to be off.
- The created file contained only two selected skill names even though four skills were selected.
- Codex `on-request` can allow normal workspace writes under `workspace-write`; it is not strict enough for FlowPilot's YOLO-off approval promise.
- Selected skill bodies were injected, but the prompt did not include a compact explicit selected-skill-name list.

### Current Ask

- Done. Codex YOLO-off now uses `untrusted`, and selected skill names are explicitly listed in the injected prompt while the desktop collapsed preview remains compact.

### Key Decisions

- `V-1` Codex YOLO-off must use `workspace-write` plus `untrusted` so workspace file writes trigger approval.
- `V-2` Codex YOLO-on remains `danger-full-access` plus `never`.
- `V-3` Selected skills must be available as an explicit name list in addition to full skill markdown content.
- `V-4` The desktop prompt skill summary may keep a compact first-two collapsed preview, but expanded state and prompt injection must preserve the full selected list.

### Constraints

- Do not weaken YOLO-on behavior.
- Preserve MCP permission response mapping from BUG-070.
- Do not delete the user's generated `yolo-test.txt` artifact unless requested.

### Open Questions

- None.

### Source Refs

- User retest on `2026-06-16`: file was created without approval UI, and file content listed only two selected skills.
- Local artifact: `yolo-test.txt` contained `yolo works`, `add-new-bug`, `add-new-task`.

## 1. Issue Summary

After the BUG-070 follow-up, Codex still created a workspace file without showing the approval UI when YOLO was expected to be off. The same run also produced only two selected skill names in the generated file despite four selected skills.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop chat mode with Codex app-server, YOLO expected off, four skills selected.
- reproduction steps:
  1. Select four skills in desktop chat.
  2. Prompt Codex to create `yolo-test.txt` in the current directory with `yolo works` and all selected skill names.
  3. Observe the file is created without approval UI.
  4. Open the file and observe only two skill names.
- frequency: reproduced in the user's retest.

## 4. Expected vs Actual

- expected: YOLO-off Codex workspace writes show approval before execution, and all selected skill names are available to the model and visible in the prompt log.
- actual: the workspace write executed without approval, and only two selected skill names appeared in the generated file.

## 5. Impact

- users affected: desktop chat users using Codex with YOLO off and selected skills.
- workflows affected: file-writing turns and skill-guided turns.
- severity: high, because YOLO-off did not enforce the expected write gate.

## 6. Root Cause

- hypothesis: the BUG-070 test matrix asserted inbound approval mapping but did not prove Codex would emit an approval for ordinary workspace writes.
- confirmed cause:
  - Codex `approvalPolicy: "on-request"` can allow ordinary workspace writes under the `workspace-write` sandbox without emitting an approval request.
  - Selected skill markdown content was injected, but the injected prompt did not include a compact explicit list of selected skill names, and the desktop summary preview hid names after the first two.
- evidence:
  - Local Codex schema lists `AskForApproval = "untrusted" | "on-failure" | "on-request" | granular | "never"`.
  - `yolo-test.txt` was created without approval and only listed two skill names.

## 7. Fix Strategy

- `F-1` Change Codex YOLO-off posture from `approvalPolicy: "on-request"` to `approvalPolicy: "untrusted"`.
- `F-2` Keep YOLO-on posture unchanged as `danger-full-access` plus `never`.
- `F-3` Add `Selected skill names: /a, /b, ...` to `injectSelectedSkills` before the skill markdown blocks.
- `F-4` Keep the desktop collapsed prompt skill summary compact while preserving all selected names in expanded state.
- `F-5` Update the Codex YOLO matrix test and selected-skill injection test.

## 8. Validation

- `V-1` `go test ./internal/runner -run "Test(ResolveYoloPosture|CodexAdapterYoloApprovalMatrix|InjectSelectedSkillsDeliversSelection|CodexAdapterApprovalRoundTrip|CodexAdapterV2ApprovalRoundTrip|CodexAdapterPermissionsApprovalRoundTrip)$" -count=1` passes.
- `V-2` `npm run typecheck --prefix apps/desktop-flowpilot` passes.
- `V-3` GitNexus impact for `resolveYoloPosture`, `Runner.injectSelectedSkills`, and `PromptSkillsSummary` reported LOW risk.

## 9. Regression Guard

- tests: `TestResolveYoloPosture`, `TestCodexAdapterYoloApprovalMatrix`, `TestInjectSelectedSkillsDeliversSelection`.
- alerts: none.
- audit checks: [CA-081](../../../change-audit/CA-081-fix-codex-yolo-off-write-gate-and-selected-skill-names.md).

## 10. Follow-Up Document Updates

- upstream docs that must change: updated Codex/YOLO notes in `requirements/10-Refactor/New-System/04-07-Operator-Docs.md`, `05-Codex-AppServer-Migration-Detail.md`, and `06-DOD-And-Verification-Checklist.md`.
- notes left unchanged on purpose:
  - Historical BUG-061 still documents why MCP TOML must use `prompt`, not task-level approval values.
