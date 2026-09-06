# BUG-360: plan-phase resume after restart dies in freeze strict-parse loop

## Metadata

- Document ID: `BUG-360`
- Title: `plan-phase resume after restart dies in freeze strict-parse loop`
- Phase: `bugfix`
- Status: `open`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-06`
- Last Updated: `2026-09-06`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [Task-325](../../08-Task/done/Task-325-Conditional-Plan-Approval-Park.md)
- Child Documents: `none`
- Related Documents: [BUG-357](../../09-BugFix/done/BUG-357-Flow-Doc-Writer-Khong-Ghi-Artifact-Instance.md)
- Replaces: `none`
- Tags: `freeze, restart-durability, planner-draft, escalate-loop`

## AI Quick View

### Summary

- Symptom (live run-584646, 2026-09-06): after reopen + approve from a plan_approval park (pre-restart), the flow advanced to `preflight_contract_freeze` and parked `escalate`: "invalid planner proposal: changecontract: strict preflight draft parse: invalid character 'c' looking for beginning of value". Every Retry re-escalates "(no progress since last continue)" — dead-end loop, no button helps except Stop.
- Root cause (reasoned, code-conformant): the planner draft lives only in the transient `preflight_contract_plan` child turn result. Post-restart the child is gone, `findPlannerResultForFreeze` finds nothing, and freeze strict-parses whatever prose fallback remains (starts with 'c') → fail. The park/approve machinery itself worked (LoopState + topology rehydrated, advance dispatched).
- Impact: any plan-phase resume after runner restart (approve OR feedback) funnels into this dead end. In-session approve is unaffected (proven separately).

### Current Ask

- Decide fix direction (see below), then implement + test. Capture-only at filing.

### Key Decisions

- D-1: Filed open without code change (same capture pattern as BUG-357 at filing).
- D-2: Approve-path live verify moves to a fresh no-restart run; run-584646 served its purpose (park trigger) and should be Stopped.

### Constraints

- additive-tests-only / oracle-rule as always.
- Must not weaken freeze strictness for the normal path (fail-closed stays).

### Open Questions

- Q-1: Preferred fix — (a) persist the planner draft JSON durably at plan time (session snapshot/file), (b) freeze falls back to deriving the draft from the plan DOC on disk (the durable `plan_md` artifact — philosophically the right source), or (c) detect-and-advise (actionable Stop guidance instead of infinite no-progress loop)? (b) is recommended: the doc outlives every restart by design.
- Q-2: Same amnesia class may affect other child-turn-result consumers post-restart (audit?). Scope check during fix.

### Source Refs

- `apps/local-runner/internal/runner/flow_validate_audit_dispatch.go:1641` (`findPlannerResultForFreeze`), `:1730-1741` (fallback), `:1680`+ (`runContractFreezeNode`)
- Live: run-584646 transcript (approve → freeze escalate ×N with "no progress" suffix); `plan_synthesis` step shows CANCELED (park-cancelled approving turn)

## Evidence

- Screenshot 2026-09-06: `blocked: escalate`, reason verbatim above, `preflight_contract_freeze WAITING_USER_APPROVAL`, Retry→error toast, repeats with "(no progress since last continue)".
- run-584646 history: parked plan_approval pre-restart (writer round ≥1, Task-910 revised plan) → reopened via `/open` (card restored, durability OK) → Retry/approve advanced past the plan gate (no re-park) → freeze escalate loop.

## Root Cause

Planner draft = transient child-turn result; restart amnesia leaves freeze with prose fallback that strict parse rejects. Park/LoopState/topology durability all held — only the draft didn't survive.

## Fix direction (proposed, NOT implemented)

- Prefer (b): freeze derives draft from the on-disk plan doc (`plan_md` binding path, resolved via BUG-357-style lookup) when the child result is absent; keep strictness when a draft IS present.
- Tests: approve-after-restart with dead planner child → freeze proceeds from doc (new file); existing freeze/restart suites green.
