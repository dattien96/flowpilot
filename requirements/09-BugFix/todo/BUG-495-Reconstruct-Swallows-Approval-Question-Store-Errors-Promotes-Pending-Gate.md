# BUG-495 — reconstructRunInternal swallows approval/question store read errors → pending gate silently promoted on resume

## Status
todo — discovered in deep-review round 2 of BUG-487..494 (same class as BUG-491, missed site)

## Severity
High — durability/resume contract

## Symptom

`reconstructRunInternal` reads the per-run approval and question sidecar
stores to decide which flow nodes must stay `waiting_approval` /
`waiting_question` on resume:

```go
if ahr, ok := s.workflowStore.(ApprovalHistoryReader); ok {
    if states, err := ahr.ListApprovalsByRun(ctx, st.RunID); err == nil {
        pendingApprovals = states
    }
}
if qhr, ok := s.workflowStore.(QuestionHistoryReader); ok {
    if states, err := qhr.ListQuestionsByRun(ctx, st.RunID); err == nil {
        pendingQuestions = states
    }
}
```

(`apps/local-runner/internal/runner/interactive_resume.go` ~:1270)

When either store read fails (corrupt sidecar, permission, torn file), the
error is silently swallowed and the pending list stays empty. The empty
lists feed `keepWaitingNodeIDsForResume`, so a node that is durably waiting
on a user approval/question is reconstructed as **not waiting** — the
resumed run promotes past an unanswered gate.

This is exactly the BUG-491 class: a store read error converted into an
"empty is complete" view feeding an authoritative derivation. The sibling
reads in the same function (`resumedFlowStepRows`,
`childPendingGateNodeIDs`) already propagate; these two were missed.

## Root cause

`err == nil` gating treats "could not read" as "nothing pending". The
contract inverts the fail direction: an unreadable approvals store must
mean "cannot prove the gate was answered → keep waiting / abort", never
"no pending gates".

## Fix (implemented)

Extract the block into `pendingGateStates(runID)` returning
`([]ProviderApprovalState, []ProviderQuestionState, error)`. Either reader
failing → error propagates to `reconstructRunInternal`, which returns
`502 gate_state_unavailable` — same fail-closed shape as the neighboring
`session_index_unavailable` arm.

## Tests

- `bug495_pending_gate_state_swallow_test.go`
  - RED: `pendingGateStates` on a faulting `ApprovalHistoryReader` → error.
  - RED: same for `QuestionHistoryReader`.
  - `reconstructRunInternal` on a run whose approvals store faults →
    `*apiErr` 502, code `gate_state_unavailable` (run does not resume
    past the unverifiable gate).
  - Healthy path: pending approval/question states still reach
    `keepWaitingNodeIDsForResume` unchanged.

## Known intermediate state

When reconstruction aborts, the partially built `rs` remains in `s.runs`
(registered before the failing read). A retry re-enters
`reconstructRunInternal`, which rebuilds and overwrites `s.runs[rs.id]`
unconditionally — the stub self-heals on the next attempt.
