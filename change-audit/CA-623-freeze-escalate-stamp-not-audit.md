# CA-623: Rag-harness freeze escalate stamped audit instead of freeze

## What

Rag-harness run-137855 (grok) parked `blocked: escalate` with gate `Contract freeze blocked: invalid planner proposal ... invalid character 'I'`. F2 showed `audit WAITING_USER_APPROVAL`, `preflight_contract_freeze`/`context`/`implement` PENDING. Banner correct, chip correct (CA-622), but the parked node was wrong. `Continue` (or click `[Continue]`) invoked the generic hub reinvoke (`I'll pick up the paused FlowPilot review`) instead of re-trying the freeze, leaving audit WAITING and freeze still PENDING.

## Why

1. Rag-harness has `hub.inline == ""` (no hub). `setFlowStepAwaitingUser` fallback searches for a `RUNNING` step else hard-falls to `audit`. At freeze-parse time no step is `RUNNING` (freeze was never marked RUNNING) and `audit` is still `PENDING`, so it stamped **audit** WAITING — `run125458_rag_harness_audit_waiting_test.go`'s `TestSetFlowStepAwaitingUser_RagHarnessNoRunning_FallbackToAudit` locked that fallback as observed behavior for the audit-RUNNING case (run-125458), but it is wrong when freeze itself failed.
2. `runContractFreezeNode` escalate closure only called the generic helper, did not `stampLastEscalatedInlineNode` and did not set `freeze` to `WAITING_USER_APPROVAL`. So `resumeFlowWithFeedback` saw `lastEscalatedInlineNodeID == ""` and fell through to `maybeAutoReinvokeHubWithNote` (parent reinvoke).
3. `applyFlowControl` `escalate` unconditionally called `setFlowStepAwaitingUser`, which then stamped audit again even after freeze was already stamped, duplicating the mis-stamp.
4. Click path was also dead (CA-622), so the only retry was `/continue` typing, which hit the same mis-routed parent.

`WAITING_USER_APPROVAL` is shared between YOLO/ask_user and escalate/cap park — chips are intentionally `Continue/Stop` not `Approve/Deny` — so the F2 label looked like a question but is a park.

## Fix

- **runner/flow_step_runtime.go** `setFlowStepAwaitingUser`: before the RUNNING→audit fallback, if **any** step is already `WAITING_USER_APPROVAL`, return early (do not stamp a second audit). Preserves run-125458: when nothing is WAITING and `audit` is RUNNING, it still stamps audit; when audit is PENDING with no RUNNING, it still falls to audit; only suppresses the duplicate when freeze was already stamped.
- **runner/flow_validate_audit_dispatch.go** `runContractFreezeNode` escalate: when flow-engine-driven, `stampLastEscalatedInlineNode(parent, node.ID)` + `setFlowStepStatus(node.ID, WAITING_USER_APPROVAL)` before `applyFlowControl`. So the helper's early-exit sees freeze WAITING and skips audit.
- **runner/flow_validate_audit_dispatch.go** `advanceFlowThroughFreezeChain` escalate: same for `nodeID` (or fallback to generic helper if empty).
- **runner/interactive_service.go** `applyFlowControl` `escalate`: if `lastEscalatedInlineNodeID != ""`, stamp that node directly instead of calling the generic helper; otherwise call `setFlowStepAwaitingUser`. Prevents the second escalate call from double-stamping audit after freeze already claimed it.
- `resumeFlowWithFeedback` hub-less path already retries `lastEscalatedInlineNodeID` via `tryAdvanceFlowThroughInline` (CA-616/617). With `lastEscalated=freeze`, Continue now re-enters freeze (via async `tryAdvance`) rather than parent. An empty feedback (`""`) re-parses and re-escalates freeze honestly (still blocked) instead of jumping to audit or parent — audit stays `PENDING`.

Keeps CA-355 review-loop hub reinvoke, CA-616 `lastFailedDelegate`, CA-617 persist, `run125458_*` audit RUNNING path, and `TestRunContractFreezeNodeRejectsInvalidDraft` loop blocked assertion.

## Tests (additive, no old edit)

- `runner/ca623_freeze_stamp_test.go` (provider-agnostic — `runContractFreezeNode`/`setFlowStepAwaitingUser`/`applyFlowControl` take no `ProviderKey`; grep `ProviderKey` 0 hits on changed symbols; rag-harness fixture is synthetic, not per-provider):
  - `TestCA623_FreezeInvalid_StampsFreezeNotAudit` — `runContractFreezeNode` with invalid JSON on `preflight_contract_freeze` → freeze `WAITING`, audit `PENDING`, `lastEscalated=freeze`, loop `blocked/escalate` with gate `Contract freeze blocked`.
  - `TestCA623_FreezeInvalid_ContinueDoesNotSpawnParentAuditStillPending` — `resumeFlowWithFeedback("")` after freeze park → audit still not WAITING, no `audit` child, loop re-blocked (async wait).
  - `TestCA623_AuditRunning_EscalateStillStampsAudit` — audit `RUNNING` then `setFlowStepAwaitingUser` → audit `WAITING` (locks run-125458 path, proves guard does not break audit escalate).
  - `TestCA623_GuardAlreadyWaiting_DoesNotStampAudit` — freeze already WAITING then second `setFlowStepAwaitingUser` → freeze stays WAITING, audit not WAITING.
  - `TestCA623_ChainEscalate_StampsChainNode` — freeze with no writer target → freeze `WAITING`.

Old: `go vet ./internal/runner` pass; `go test ./internal/runner -run TestCA623|TestSetFlowStepAwaitingUser|TestRunContractFreeze|TestInlineChain|TestRun135037|TestRun125458` pass.

Provider parity: **Case 1 agnostic** — confirmed provider-agnostic with grep/read; no `ProviderKey` branching. Synthetic topology exercises rag-harness shape; running-child-wins invariant kept via `hasLiveWorkingChild`.

Will not undo: CA-616/617 `lastFailedDelegate`/`lastEscalated` persist, CA-619 escalate park Thinking/chip, CA-620 sendBlocked, run-125458 audit WAITING when audit really is the escalated node.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-43
change_type: bugfix
summary: freeze escalate stamps freeze WAITING not audit — guard duplicate WAITING and stamp lastEscalated so Continue re-enters freeze (CA-623)
# --->8---
