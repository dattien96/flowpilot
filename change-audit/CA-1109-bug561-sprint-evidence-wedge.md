# CA-1109 — BUG-561: audit sprint-evidence defer for in-flight legs + Continue routes to the remediation hub

## Why

Live run-60899 (Task-023 sprint): the audit node evaluated
`blocked_missing_feature_key` with `openIssues=2` while the round-1
reviewer leg was still in flight. BUG-560 covered the cohort-member shape,
but two adjacent gaps remained:

1. Vibe coder/reviewer legs are often hub ad-hoc `spawn_agent wait=true`
   children with `flow_cohort_id=""` — `hasOpenCohort` misses them, so a
   not-ready audit could still escalate → park → cancel the leg mid-run.
2. Once escalated (e.g. after the adjudicating member died), the park's
   Continue re-entered the audit node on identical state and re-escalated
   forever — a guaranteed wedge requiring a manual `flow-control` poke.

## What changed

- `flow_validate_audit_dispatch.go`:
  - `runAuditNode`'s not-ready defer now also fires when any non-audit flow
    step is `RUNNING` under vibe working mode (a live leg owed a terminal
    transition whose settle re-drives the flow and re-dispatches audit).
  - New helper `hasRunningSprintStep(parentRunID, excludeID)`.
- `interactive_service.go`:
  - New matcher `isSprintEvidenceIncompleteReason` for the audit's
    "sprint evidence incomplete" gate reason.
  - `resumeFlowWithFeedback`: on an escalate park with that reason and the
    escalated node being `artifact.audit_draft`, Continue routes to the
    sprint hub (`maybeAutoReinvokeHubWithNote`) with an explicit remediation
    directive instead of re-entering the audit node. Hub-less flows keep
    the existing re-entry fall-through.

## Evidence

- `bug561_audit_sprint_evidence_wedge_test.go`:
  - `TestBug561_AuditDefersWhileUpstreamStepRunning` — RUNNING `validate`
    step, no cohort: audit defers to PENDING, loop stays running.
  - `TestBug561_AuditEscalatesWhenNoStepInFlight` — all legs terminal +
    open issues: escalate contract unchanged.
  - `TestBug561_SprintEvidenceContinueReinvokesHub` — parked wedge +
    bare Continue produces a hub turn carrying the remediation note,
    across claude/codex/grok.

## Verification

- `go test ./internal/runner -run 'TestBug560|TestCA1096|TestCA1098|
  TestBug561|TestCA1074|TestBug550|TestRunContractFreeze|TestBug505|
  TestBug360|Test.*Cohort|Test.*Reinvoke'` — green.
- Full `./internal/runner` suite: only the known pre-existing failures
  (TestBug334 ×3, environmental sessions/grok/firebase set) plus the
  gitnexus auto-index `TempDir` cleanup flake (passes on re-run).
