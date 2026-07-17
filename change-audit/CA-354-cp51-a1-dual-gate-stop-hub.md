# CA-354 — CP-51 A1 live: dual gate UI, Continue hang, Stop hub

## Context

Live desktop A1 on gate-sandbox (`run-10389` hub + `run-10394` coder) showed:

1. Two decision surfaces at once (flow escalate card + regression modal), twice.
2. Continue after escalate failed with `hub_reinvoke_start_failed: post-turn gate still running`.
3. Stop on main/hub felt like a no-op; Stop on focused child worked better.

Evidence and checklist: `requirements/07-Coding-Plan/inprogress/CP-51-PhaseAB-Timeline-And-Verification-Log.md` §3.6 + §5.

## Changes

### Runner

- `gate_hook.go` — child coding gate **block with r-reg options** no longer also escalates the hub (avoids dual UI + Continue-while-child-gate-open).
- `interactive_service.go` `resumeFlowWithFeedback` — clear **stale** hub `pendingFlowGateSettle` when `postTurnGateCancel == nil` so Continue can reinvoke.
- `interactive_handlers.go` `handleStopAgentLoop` — on durable fence/persist failure still return graph **snapshot** in the error JSON body so UI can settle.

### Desktop

- `ChatInput.tsx` — Stop button also when `status === "blocked"`.
- `FlowAwaitingUserCard.tsx` — hide while `gateBlock` modal is open.
- `store.ts` `stop()` — always clear `gateBlock`; broaden when to call `stopAgentLoop` (snapshot / agent children / workflow_step_auto); try/catch with embedded snapshot; interrupt **all** children.
- `HttpWsRunnerClient.ts` — `RunnerApiError.snapshot` from error body.

## Out of scope

- Full multi-violation messaging when only regression modal is shown (next gate turn re-checks audit/contract).
- Automatic grant of write permission for `calc.go`.

## Verify

```bash
cd apps/local-runner
go test ./internal/runner/ -count=1 -timeout 3m -run 'TestStopAgentLoop|TestCancelPendingGates|TestOwnRunStop|TestStopCAS'
```

Retest live A1 per CP-51-VERIFY §3.6.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: bugfix
summary: Fix A1 dual gate UI, hub Continue gate_in_progress hang, and weak Stop on main/hub
# --->8---
