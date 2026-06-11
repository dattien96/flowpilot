# Task-032: Document Proxy YOLO And Provider Approval Config

## Metadata

- Document ID: `Task-032`
- Title: `Document Proxy YOLO And Provider Approval Config`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-11`
- Last Updated: `2026-06-11`
- Parent Documents: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`, `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`, `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- Child Documents: `none`
- Related Documents: `requirements/09-BugFix/done/BUG-032-Codex-Google-Drive-MCP-Model-And-Approval.md`, `requirements/08-Task/done/Task-031-Single-Step-Step-Definition-Yolo-Policy.md`, `change-audit/CA-043-cp29-google-drive-mcp-manual-approval-ui.md`
- Replaces: `none`
- Tags: `google-drive-mcp, yolo, codex, provider-config, proxy, docs`

## AI Quick View

### Summary

- Current FlowPilot behavior depends on a deliberate split between provider-host approval and FlowPilot-owned approval.
- For Codex Google Drive MCP config, the generated provider block must contain `default_tools_approval_mode = "approve"` so headless runs do not hang before FlowPilot can inspect the request.
- YOLO is not implemented by rewriting provider-host approval into per-run values; provider-host approval is kept non-blocking and the real policy is enforced in the FlowPilot proxy/runtime layer.
- The current system already rewrites the Codex Google Drive MCP config when provider setup runs, but this behavior is not documented clearly enough for operators.

### Current Ask

- Record the exact current behavior for proxy YOLO versus provider approval config, and define follow-up improvements so the setup/runtime surfaces make this behavior explicit and diagnosable.

### Key Decisions

- `T-1` Keep the documentation explicit that provider-host approval and FlowPilot approval are different layers.
- `T-2` Treat `default_tools_approval_mode = "approve"` in the generated Codex Google Drive MCP block as required runtime wiring, not as the source of truth for YOLO policy.
- `T-3` Document that provider config repair happens only when FlowPilot reaches `EnsureGoogleDriveMcpProviderConfig(...)`, not during unrelated app usage.
- `T-4` Improvements should focus on visibility, diagnostics, and self-heal confidence before any broader policy refactor.

### Constraints

- No code change is part of this task.
- Do not redefine CP-29 approval semantics in a downstream task note.
- Keep the scope on Google Drive proxy MCP provider config, YOLO interpretation, and operator-facing understanding.

### Open Questions

- Should FlowPilot expose a dedicated provider-config health check that fails loudly when Codex approval mode drifts away from `approve`?
- Should Step 7 show both the provider config file path and a strong warning when the discovered approval mode is missing or not `approve`?
- Should runtime log an explicit self-heal event whenever `EnsureGoogleDriveMcpProviderConfig(...)` rewrites a stale provider block?

### Source Refs

- `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- `requirements/09-BugFix/done/BUG-032-Codex-Google-Drive-MCP-Model-And-Approval.md`
- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
- `apps/admin-web/src/routes/_authenticated/settings/components/-GoogleDriveProviderConfigCard.tsx`
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`

## 1. Goal

Document the current Google Drive proxy MCP approval architecture clearly enough that another engineer or AI agent can answer these questions without re-debugging the code:

- why Codex provider config needs `default_tools_approval_mode = "approve"`
- where YOLO is actually enforced
- when FlowPilot rewrites stale provider config automatically
- what gaps remain in setup/runtime visibility

## 2. Parent Links

- coding plan: `requirements/07-Coding-Plan/priority/CP-29-MCP-Proxy-Google-Drive.md`
- tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`, `requirements/06-System-Tech-Design/SD-09-Approval-Gates.md`
- system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`, `requirements/05-System-Specs/SS-08-Approve-Gate.md`
- specific upstream ids:
  - `CP-29 P-5` FlowPilot proxy approval requires user approval when YOLO is off
  - `CP-29 P-11` FlowPilot proxy approval auto-approves policy-allowed calls when YOLO is on
  - `CP-29` constraint that provider-host approval must stay non-blocking for headless runs

## 3. Trigger

Operator review of the current Google Drive setup and workflow runtime behavior exposed a documentation gap:

- Codex can hang if its provider config does not contain the generated `default_tools_approval_mode = "approve"` line.
- The current code already restores that line when provider config ensure runs, but this is easy to misunderstand because YOLO policy is enforced elsewhere.
- The setup page and current docs do not explain strongly enough that:
  - provider-host approval is bypassed on purpose
  - FlowPilot proxy approval is the real gate
  - self-heal only runs on specific setup/runtime paths

## 4. Exact Change

- `T-1` Add or update a task/design note that explains the two-layer approval model:
  - provider-host approval config
  - FlowPilot proxy/runtime approval policy
- `T-2` Document the Codex-specific requirement that generated Google Drive MCP config must include `default_tools_approval_mode = "approve"` so provider-host prompts do not block headless execution before the request reaches FlowPilot.
- `T-3` Document the exact repair behavior:
  - Step 7 `Configure AI Providers` rewrites stale provider config
  - runtime before a Google Drive MCP step also calls `EnsureGoogleDriveMcpProviderConfig(...)`
  - unrelated FlowPilot usage does not touch provider config
- `T-4` Capture follow-up improvements for a later implementation slice:
  - stronger Step 7 warning when approval mode is missing or wrong
  - explicit runtime/self-heal logs when provider config is repaired
  - a dedicated health/preflight check for provider approval-mode drift
  - clearer provider-row UI text that this setting is runtime wiring, not the YOLO source of truth

## 5. Touched Areas

- files:
  - `requirements/08-Task/todo/Task-032-Document-Proxy-Yolo-And-Provider-Approval-Config.md`
- modules:
  - Google Drive proxy MCP provider config generation
  - workflow runtime Google Drive MCP config ensure path
  - setup-page provider configuration UI
- routes:
  - `/_authenticated/settings/google-drive-setup`
- tables:
  - none

## 6. Acceptance Check

- The task note must explain that Codex `default_tools_approval_mode = "approve"` is required so requests reach FlowPilot instead of hanging in provider-host approval.
- The task note must explain that FlowPilot YOLO is enforced in proxy/runtime policy, not by toggling provider-host config per run.
- The task note must explicitly state when automatic provider-config repair does and does not happen.
- The task note must capture at least one concrete follow-up improvement for diagnostics or self-heal visibility.

## 7. Out of Scope

- Changing provider config generation logic.
- Changing Codex, Gemini, or Claude runtime semantics.
- Changing CP-29 approval rules or YOLO policy behavior.
- Implementing any new Step 7 warnings, runtime logs, or health checks in this task.

## 8. Completion Notes

- result: `planned only; no code or upstream document updates yet`
- follow-ups:
  - create an implementation task if we want Step 7 and runtime to surface provider approval-mode drift more clearly
  - consider a focused test/diagnostic slice that proves stale Codex provider config is always repaired before Google Drive MCP execution
- upstream docs updated: `none`
