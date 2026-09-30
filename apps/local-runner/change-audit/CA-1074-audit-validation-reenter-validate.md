# CA-1074: validation-blocked audit escalate must re-enter `command.validate`, not re-invoke the synthesis hub

Date: 2026-10-01
Refs: CP-90 (`vibe-tasks` entry), CA-1072 (existing-Tasks entry),
CA-1073 (drift-debate discharge), run-202550 (missing-CA remediation
pattern), BUG-362 (stamp the escalated node, not first-hub fallback),
BUG-289 (hub-less Continue), live lanes of `TestVibeTasksLive` on
devin/swe-2-high.

## Problem

Across **all 6 live `vibe-tasks` lanes**, sprint-1's flow jumped
`tdd → synthesis → audit`: the `coder` and `validate` flow nodes never
stamped a transition (0 hits in every workspace's
`run-1-step-transitions.ndjson`). Real code work happened anyway — debate
verdicts reprompt coder children outside the flow graph — so
`flowValidationRetryState` stayed empty forever.

The audit node then built `blocked_validation_failed` and escalated:
`"Audit blocked: validation was not positively verified (status=blocked_validation_failed, validation=)."`.
But the escalate path parked with **no
`lastEscalatedInlineNodeID`** (`setFlowStepAwaitingUser` stamps the first
hub node WAITING but records no escalated node). Every operator/test-driver
Continue therefore fell through to the generic hub reinvoke — re-running
`synthesis → audit` on the same stale validation state, ~10 min of real LLM
turns per round, forever. Sprint-1 could never terminate, so the sprint
boundary never fired and every lane starved at `chain starved: 1/N`.

## Solution

Two coordinated changes, both extending the existing run-202550 seam (no
new persistence, no parallel routing):

1. `runAuditNode` blocked-not-ready escalate
   (`flow_validate_audit_dispatch.go`): park the **audit node itself**
   `WAITING_USER_APPROVAL` (BUG-362 semantics — not the first-hub
   fallback) and stamp `lastEscalatedInlineNodeID = audit`, so Continue
   can route precisely instead of falling through to hub reinvoke.

2. `resumeFlowWithFeedback` (`interactive_service.go`): when the
   escalated node is `artifact.audit_draft` and the gate reason is the
   validation-not-verified block, walk forward edges backwards to the
   nearest upstream `command.validate` node and re-enter it via the
   existing `tryAdvanceFlowThroughInline` seam. The validate node re-runs
   the suite and persists a fresh `FlowValidationRetryState`; the flow
   then advances `validate → synthesis → audit` on its own edges — no
   state-machine bypass, audit stays fail-closed (a re-run that fails
   escalates from validate, not audit). Topologies with no upstream
   validate fall through to the pre-existing audit re-dispatch.

Helper refactor: `upstreamCodeWriterForNode` becomes a thin wrapper over
the new generic `upstreamNodeByBehavior(edges, nodes, nodeID, canonical)`
backward-walk; the run-202550 missing-CA writer lookup is unchanged.

## Files

- `internal/runner/flow_validate_audit_dispatch.go` — stamp audit node +
  park audit WAITING on blocked-not-ready escalate.
- `internal/runner/interactive_service.go` —
  `isValidationNotVerifiedReason`, `upstreamNodeByBehavior` (generic
  walk; `upstreamCodeWriterForNode` delegates), and the
  validation-blocked Continue branch before the writer-park dispatch.
- `internal/runner/ca1074_audit_blocked_validation_reenter_test.go`
  (new): reproduce-first red tests —
  `TestCA1074AuditValidationBlockedStampsAuditNode` (real `runAuditNode`
  escalate stamps/parks `audit`, NOT the `synthesis` hub) and
  `TestCA1074ValidationBlockedContinueReentersValidate` (Continue
  re-enters `command.validate`; `flowValidationRetryState` records a real
  `passed` verdict instead of staying empty). 3-provider matrix
  (claude/codex/grok) — the path is provider-agnostic.
- `internal/runner/cp61_hub_done_verdict_test.go`,
  `internal/runner/run207435_plan_approval_no_repark_after_freeze_test.go`:
  assertion corrections — both asserted `audit ∈ {RUNNING, DONE}` after
  dispatch, which encoded the pre-fix mis-stamp (audit stayed RUNNING
  while the hub took the WAITING stamp). Since `applyFlowControl`
  re-stamps `lastEscalatedInlineNodeID` WAITING, a dispatched audit that
  blocks is now correctly `WAITING_USER_APPROVAL` — the same shape the
  run-202550 tier-3 block already used. The accepted set is extended with
  `WAITING_USER_APPROVAL`; the protective intent (audit dispatched, not
  PENDING/undispatched) is unchanged.

## Verification

- Red→green: both tests failed before the fix (stamp `""`, validate never
  re-entered) on all three providers; pass after.
- `go test -count=1 -run 'TestCA1074' ./internal/runner/` — ok.
- `go test -count=1 -run 'TestRun202550|TestRun147126|Audit|AuditDraft|
  Resume' ./internal/runner/` — ok; run-202550 missing-CA remediation
  path unchanged (writer lookup preserved).
- Full `internal/runner` suite: only pre-existing env-dependent FAILs
  (`codex`/`agy` binaries absent from PATH, Supabase/Firebase MCP, TempDir
  teardown flakes) — identical on the pre-change baseline; the 3 names not
  covered by the baseline subset re-verified green in isolation.
- **Live `TestVibeTasksLive` (devin/swe-2-high, 2-task bed): PASSED in
  ~9.4 min.** First lane ever to stamp a `validate` transition:
  `audit RUNNING → audit WAITING_USER_APPROVAL → synthesis DONE →
  validate RUNNING → validate DONE → synthesis RUNNING` — Continue
  re-entered `command.validate`, recorded `passed`, and the flow walked
  `validate → synthesis → audit → boundary` on its own edges. Both sprint
  children triggered in order (`Task-21`, `Task-22`), `vibeTaskIndex=2`.
  Prior lanes all starved `chain starved: 1/N` at the 90-min mark.

## Live evidence (pre-fix)

Every live workspace showed the identical wedge: `tdd RUNNING → synthesis
RUNNING → audit RUNNING` transitions with **zero** `coder`/`validate`
entries, gate reason pinned at `validation was not positively verified`,
and each Continue re-entering `synthesis` — including one lane where the
hub correctly reported "all tasks implemented, tests green" while the
graph had no validation record, so the audit could never settle.
