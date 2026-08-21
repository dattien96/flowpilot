# CA-585: Show validate/audit as RUNNING before command execution

## What

Rag-harness F1: `implement` DONE but next steps `validate`/`audit` never appeared as `[RUNNING]` in chat or F2 before `go test` / audit draft started — they stayed `[ ]` pending while the flow was actually running, and `Thinking` kept spinning on the hub with no visible step. User saw `+ 2. [DONE] implement` and frozen `[✓] validate`/`[✓] audit`.

## Why

- `flow_validate_audit_dispatch.go:388` `runValidateNode` and `:779` `runAuditNode` only wrote terminal `DONE`/`FAILED` after the command/draft finished. No `setFlowStepStatus(..., RUNNING)` was issued at entry, unlike the three `tryAdvanceFlowFromNode` delegate paths (`flow_executor.go:210,524,1055`) which stamp `RUNNING` immediately. The TUI's `formatStepChatNotices` only emits `> N. [RUNNING] validate` when `flowStepsActive` changes to `RUNNING`, so inline steps were silent until completion.
- Pending default glyph (pre CA-584) hid the issue further by rendering pending as done.

## Fix

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:455` — after `skipped_no_command` escalation check, if `isFlowEngineDriven` then `setFlowStepStatus(ctx, parentRunID, node.ID, StepStatusRunning)` before `ensureBaselineReadyContext` / `RunValidationCommand`.
- Same at `:812` for `runAuditNode` before observation + `BuildAuditDraft`.
- Provider-agnostic: `command.validate` / `artifact.audit_draft` are inline behaviors with no `providerKey` branch; no per-provider code.

## Tests

- Existing `go test ./internal/runner -run "TestValidate|TestAudit|TestFlow" -count=1` 24s green (no old test edited).
- New behavior is covered by the TUI's pending→running transition: with the fix, a slow `go test` will show `> 3. [RUNNING] validate` in chat and F2 spinner before completion; previously it stayed `[ ]` until `DONE`.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-56
change_type: bugfix
summary: mark validate/audit steps RUNNING before execution so F2 and chat show live progress
# --->8---
