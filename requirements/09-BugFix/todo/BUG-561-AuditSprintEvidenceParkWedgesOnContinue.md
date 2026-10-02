# BUG-561 — Audit "sprint evidence incomplete" park wedges; Continue re-enters the audit forever

- **ID:** BUG-561
- **Severity:** High (flow wedge — bare Continue can never make progress; live run-60899 needed a manual `flow-control` poke with `issues:[]` to unblock)
- **Status:** RESOLVED — CA-1109
- **Found:** live run-60899 (Task-023 sprint, 2026-10-02 ~10:32)

## Symptom

Vibe sprint `audit` node evaluated `blocked_missing_feature_key` with
`vibeSprintEvidenceComplete=false` (`loop_open_issues=2` from the prior
review verdict) while the round-1 reviewer (`run-67320`, cohort
`flow-auto-validate-round-1`) was still in flight. The escalate parked the
flow and the park cancelled the reviewer mid-turn (`interrupted by user` —
fixed by BUG-560). With the member dead and drained, `OpenIssues=2` became
permanently stale: no verdict could ever land to re-adjudicate it.

Every operator Continue then re-entered the audit node itself
(`lastEscalatedInlineNodeID=audit` → `flowNodeInlineDispatchable` →
`tryAdvanceFlowThroughInline`), which re-evaluated the identical evidence
and re-escalated with the same reason — a guaranteed re-park loop. The run
only recovered after an out-of-band `flow-control continue` with
`issues:[]` plus a second `agent-loop/continue`.

## Root cause

Two gaps around the same missing-evidence class:

1. **Defer coverage is cohort-only.** BUG-560 made the audit defer while an
   upstream cohort barrier is open, but hub ad-hoc legs (vibe sprint coder
   children are `spawn_agent wait=true` hub calls — `flow_cohort_id=""`) are
   not cohort members. A not-ready audit evaluated while a non-cohort
   leg is still RUNNING can still escalate → park → cancel that leg — the
   exact BUG-560 kill pattern through a different spawn shape. The flow
   already tracks leg liveness on the step timeline (`reviewer` step was
   RUNNING at the live escalate): a RUNNING non-audit step is owed a
   terminal transition that re-drives the flow.

2. **The park's Continue has no remediation owner.** `resumeFlowWithFeedback`
   special-cases CA-1074 (validation-not-verified → re-enter upstream
   `command.validate`) and CA-1098 (verdict-gate escalate → re-drive
   deficient cohort members), but the sprint-evidence-incomplete reason
   falls through to the generic inline re-entry, which re-runs the audit on
   byte-identical state. The sprint hub (`synthesis`, not sealed by
   `vibeHubSealed`) is the component that owns remediation — it spawned the
   remediation legs ad-hoc and it alone can emit `continue` with issues and
   re-drive the coder, or `done` to clear the loop forward.

## Fix

- `runAuditNode`: extend the not-ready defer to also fire while any
  non-audit flow step is `RUNNING` (vibe working mode), not only while a
  cohort is open. Same defer contract as BUG-560 — audit step back to
  PENDING, handled, re-dispatched by the leg's settle → join/advance edges.
- `resumeFlowWithFeedback`: on `prevBlockReason=="escalate"` with a
  sprint-evidence-incomplete gate reason and the escalated node being
  `artifact.audit_draft`, route Continue to the sprint hub reinvoke with an
  explicit remediation note (re-drive the deficient leg / re-adjudicate)
  instead of re-entering the audit node. Hub-less flows keep the existing
  fall-through.

## Regression tests

- `bug561_audit_sprint_evidence_wedge_test.go`
  - audit defers (PENDING, loop not blocked) while a non-cohort sprint step
    is RUNNING — same contract as the BUG-560 cohort defer.
  - parked sprint-evidence escalate + bare Continue re-invokes the hub turn
    (not an audit re-entry that re-parks identically).

## Live evidence

- run-60899 log: `loop_open_issues=2` set 09:45:39 (round-1 continue with 2
  issues); reviewer `run-67320` RUNNING 10:31:52; audit RUNNING 10:32:09 →
  `WAITING_USER_APPROVAL` → `flow_control_escalate` → `park_cohort_seats_released`
  → `cohort_member_failed … error="interrupted by user"`; reviewer step
  FAILED at 10:32:09.72 while `open_issues` stayed 2.
