# CA-081: Fix Codex YOLO-Off Write Gate And Selected Skill Names

## Scope

- Codex YOLO posture in `apps/local-runner/internal/runner/yolo_resolver.go`.
- Selected skill prompt injection in `apps/local-runner/internal/runner/runner.go`.
- Desktop prompt skill summary rendering in `apps/desktop-flowpilot/src/components/Timeline.tsx`.
- Regression tests in `apps/local-runner/internal/runner`.

## Completed

- Changed Codex YOLO-off approval policy from `on-request` to `untrusted` so ordinary workspace file writes require approval.
- Kept YOLO-on as `danger-full-access` plus `never`.
- Added an explicit selected-skill-name line before injected skill markdown content.
- Kept the desktop collapsed prompt skill summary compact, while preserving all selected skill names in the expanded summary and prompt injection.
- Updated BUG-070 wording and added BUG-071 for the retest regression.

## Verification

- `npx gitnexus impact resolveYoloPosture --direction upstream` reported LOW risk.
- `npx gitnexus impact injectSelectedSkills --direction upstream` reported LOW risk.
- `npx gitnexus impact PromptSkillsSummary --direction upstream` reported LOW risk.
- `go test ./internal/runner -run "Test(ResolveYoloPosture|CodexAdapterYoloApprovalMatrix|InjectSelectedSkillsDeliversSelection|CodexAdapterApprovalRoundTrip|CodexAdapterV2ApprovalRoundTrip|CodexAdapterPermissionsApprovalRoundTrip)$" -count=1` passed.
- `npm run typecheck --prefix apps/desktop-flowpilot` passed.

## Residual Notes

- The user-generated `yolo-test.txt` remains in the workspace and was not deleted.
- A full `go test ./internal/runner -count=1` was not rerun after this patch; the previous broad run still failed on unrelated Google Drive config/provider-home/registry/history tests.

# ---8<--- flowpilot:change-ledger
feature_key: yolo-policy
source_doc_id: BUG-070
change_type: fix
summary: Fix Codex YOLO-Off Write Gate And Selected Skill Names
# --->8---
