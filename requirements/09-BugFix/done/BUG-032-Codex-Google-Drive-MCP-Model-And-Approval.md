# BUG-032: Codex Google Drive MCP Model Override And Tool Approval

## Metadata

- Document ID: `BUG-032`
- Title: `Codex Google Drive MCP Model Override And Tool Approval`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `FlowPilot`
- Created: `2026-06-08`
- Last Updated: `2026-06-08`
- Parent Documents: `CP-05-03`
- Child Documents: `none`
- Related Documents: `requirements/07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md`
- Replaces: `none`
- Tags: `google-drive-mcp, codex, provider-runtime, regression, phase-b`

## AI Quick View

### Summary

- Codex Google Drive MCP Phase B testing exposed two provider-runtime bugs.
- The first Codex MCP tool call could inherit account config `model = "gpt-5.5"` even when FlowPilot selected `gpt-5.4-mini`.
- Read-only Google Drive MCP tools could block on interactive Codex approval because generated config used `default_tools_approval_mode = "prompt"`.
- Both issues made provider-driven MCP workflow testing fail or hang despite valid Google Drive auth.

### Current Ask

- Record the two Phase B bugs and the implemented fixes.

### Key Decisions

- `V-1` FlowPilot must pass the selected step model into both Codex MCP process config and the initial Codex MCP tool-call payload.
- `V-2` Read-only Google Drive MCP tools must be auto-approved when they are constrained by an allowlist.
- `V-3` Write/destructive Google Drive MCP tools must remain behind a separate FlowPilot-owned approval model.

### Constraints

- Do not rely on interactive Codex approval prompts from programmatic `codex mcp-server` execution.
- Preserve user-owned Codex config outside the generated `google-drive` MCP server block.
- Existing Codex MCP threads may keep the model they started with; runtime validation must use a fresh session.

### Open Questions

- How should FlowPilot represent scoped write approvals for future Google Drive MCP write tools?

### Source Refs

- Phase B test notes: `requirements/07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md`
- Observed Codex config: top-level `model = "gpt-5.5"` and `model_reasoning_effort = "high"`
- Observed approval prompt: `Called google-drive.authGetStatus({})`

## 1. Issue Summary

During Phase B Google Drive MCP testing, Codex could launch with the correct FlowPilot command but still fail with a `gpt-5.5` model error. After that was fixed, the provider-driven MCP run could still hang because Codex requested interactive approval before calling read-only Google Drive MCP tools.

## 2. Parent Links

- impacted coding plan: `requirements/07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md`
- impacted tech design: `requirements/06-System-Tech-Design/SD-11-MCP-Connection-Flows.md`
- impacted system spec: `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md`

## 3. Environment and Reproduction

- environment: local FlowPilot runner using Codex provider with Google Drive MCP configured through `@piotr-agier/google-drive-mcp`
- reproduction steps:
  1. Configure Google Drive MCP auth and Codex provider config.
  2. Run a Phase B workflow step with `requiredMcps: ["google_drive"]` and selected model `gpt-5.4-mini`.
  3. Observe Codex MCP using or reporting `gpt-5.5`, or waiting for approval on `google-drive.authGetStatus({})`.
- frequency: reproducible when the Codex account config default model was `gpt-5.5` or MCP tool approval mode was `prompt`.

## 4. Expected vs Actual

- expected: Codex MCP should use the selected FlowPilot step model and call allowlisted read-only Google Drive MCP tools without interactive approval.
- actual: Codex MCP could inherit account default `gpt-5.5`, and read-only MCP calls could require an approval prompt FlowPilot could not answer.

## 5. Impact

- users affected: users testing or running workflow steps that require Google Drive MCP through Codex.
- workflows affected: Phase B provider-driven Google Drive MCP runtime validation and workflow execution with `requiredMcps`.
- severity: high for Phase B because it blocks reliable provider-side MCP verification.

## 6. Root Cause

- hypothesis: FlowPilot only configured the outer Codex MCP server process and did not control the actual initial Codex tool call or per-tool approval mode.
- confirmed cause:
  - The initial MCP `codex` tool call did not include the selected model, so Codex could fall back to top-level account config.
  - Generated Codex Google Drive MCP config used `default_tools_approval_mode = "prompt"` for read-only tools.
- evidence:
  - Local Codex MCP schema shows the `codex` tool accepts an optional `model` argument and `config` overrides.
  - Manual Codex test showed Google Drive auth was valid, but read-only tool execution required an approval prompt.

## 7. Fix Strategy

- `F-1` Add selected model and `model_reasoning_effort` to Codex MCP process startup overrides.
- `F-2` Add selected model and `config.model_reasoning_effort` to the initial MCP `codex` tool-call arguments.
- `F-3` Generate Codex Google Drive MCP read-only config with `default_tools_approval_mode = "approve"` while keeping the read-only `enabled_tools` allowlist.
- `F-4` Keep non-read-only/write behavior separate from this fix and require a future FlowPilot-owned approval model.

## 8. Validation

- `V-1` Focused runner tests confirm Codex MCP startup includes the selected model and reasoning config.
- `V-2` Focused runner tests confirm the initial Codex MCP tool call includes the selected model and `model_reasoning_effort`.
- `V-3` Provider config tests confirm read-only Codex Google Drive MCP config uses approval mode `approve`.
- `V-4` Manual Codex test confirms Google Drive auth status is valid through `google-drive.authGetStatus`.

## 9. Regression Guard

- tests:
  - `TestStartSessionCodexMcpUsesRequestedModelConfig`
  - `TestSendMessageInjectsRequiredGoogleDriveInstructionsIntoActualPrompt`
  - `TestEnsureCodexGoogleDriveMcpConfig`
- alerts:
  - Surface provider command and session command logs in workflow run detail.
- audit checks:
  - Confirm generated Codex config contains `enabled_tools` and `default_tools_approval_mode = "approve"` for read-only mode.
  - Confirm fresh Codex MCP sessions are used after changing model config.

## 10. Follow-Up Document Updates

- upstream docs that must change:
  - `requirements/07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md`
- notes left unchanged on purpose:
  - Write/destructive Google Drive MCP tools remain out of scope until FlowPilot has scoped approval storage and enforcement.
